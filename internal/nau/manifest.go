package nau

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"path"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/s3client"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
)

// Pre-created errors for easier testing and readability
var (
	ErrHeaderNotWritten = errors.New("manifest header not written")
	ErrManifestClosed   = errors.New("manifest closed - cannot write more records")
	ErrAlreadyFinalized = errors.New("manifest already finalized")
)

// CSVRecord represents a data structure that can be exported as a CSV row.
type CSVRecord interface {
	Header() []string // Returns column headers
	Values() []string // Returns row values
}

// ErrorHandler is a callback function for handling errors during manifest operations.
type ErrorHandler func(error)

// Manifest provides functionality to buffer CSV records and upload them to S3.
// Records are accumulated in memory and written as a single CSV file on finalization.
type Manifest interface {
	// WriteRecord buffers a CSVRecord for later CSV generation.
	WriteRecord(rec CSVRecord) error
	// Finalize generates the complete CSV and uploads it to S3.
	Finalize() error
}

// manifestImpl implements the Manifest interface with S3 upload capability.
type manifestImpl struct {
	parentCtx  context.Context    // Parent context (e.g., Lambda context)
	ctx        context.Context    // Manifest's own context
	cancel     context.CancelFunc // Cancel function for manifest context
	bucket     string             // S3 bucket name
	key        string             // S3 object key
	client     s3client.S3Client  // S3 client for uploads
	header     []string           // CSV header columns
	records    [][]string         // Buffered CSV records
	errHandler ErrorHandler       // Error handling callback
	log        logger.Logger      // Logger instance

	// Thread-safety components
	recordChan   chan CSVRecord
	processorWg  sync.WaitGroup
	finalizeOnce sync.Once // Ensures Finalize runs only once
}

// NewManifest creates a new Manifest instance for CSV generation and S3 upload.
// The manifest will buffer records and upload them to the specified S3 location on finalization.
func NewManifest(parentCtx context.Context, bucket, key string, client s3client.S3Client, errHandler ErrorHandler, log logger.Logger) Manifest {
	metadata := ResourceMetadata{}

	// Create manifest's own context derived from parent
	ctx, cancel := context.WithCancel(parentCtx)

	m := &manifestImpl{
		parentCtx:  parentCtx,
		ctx:        ctx,
		cancel:     cancel,
		bucket:     bucket,
		key:        key,
		client:     client,
		errHandler: errHandler,
		header:     metadata.Header(),
		log:        log,
		recordChan: make(chan CSVRecord, 1000),
	}

	// Start the processor immediately
	m.processorWg.Add(1)
	go m.recordProcessor()

	m.log.Info("manifest initialized for S3 upload: %s", key)
	return m
}



// WriteRecord adds a CSV record to the buffer for later processing.
// This method is thread-safe and uses internal channels for coordination.
func (m *manifestImpl) WriteRecord(rec CSVRecord) error {
	// Check context first for fail-fast behavior
	select {
	case <-m.ctx.Done():
		return ErrManifestClosed
	default:
	}
	
	// Context is not cancelled, try to send record
	select {
	case m.recordChan <- rec:
		return nil
	case <-m.ctx.Done():
		return ErrManifestClosed
	}
}

// recordProcessor handles records sequentially in a separate goroutine.
func (m *manifestImpl) recordProcessor() {
	defer m.processorWg.Done()
	for {
		select {
		case rec, ok := <-m.recordChan:
			if !ok {
				// Channel closed - exit immediately
				return
			}
			vals := append([]string(nil), rec.Values()...)
			m.records = append(m.records, vals)
		case <-m.ctx.Done():
			// Context cancelled - drain remaining records then exit
			m.drainRecords()
			return
		}
	}
}

// drainRecords processes any remaining records in the channel before shutdown.
func (m *manifestImpl) drainRecords() {
	for {
		select {
		case rec, ok := <-m.recordChan:
			if !ok {
				return // Channel closed
			}
			vals := append([]string(nil), rec.Values()...)
			m.records = append(m.records, vals)
		default:
			// No more records available
			return
		}
	}
}

// Finalize generates the complete CSV from buffered records and uploads to S3.
// This operation is idempotent - subsequent calls are no-ops.
func (m *manifestImpl) Finalize() error {
	var finalizeErr error

	m.finalizeOnce.Do(func() {
		// Signal shutdown and close the record channel
		m.cancel() // Cancel manifest context
		close(m.recordChan)

		// Wait for processor to finish
		m.processorWg.Wait()

		// Build CSV in memory
		buf := &bytes.Buffer{}
		csvw := csv.NewWriter(buf)
		if err := csvw.Write(m.header); err != nil {
			m.handleErr(err)
			finalizeErr = err
			return
		}

		m.log.Info("record count : %d", len(m.records))
		for _, row := range m.records {
			if err := csvw.Write(row); err != nil {
				m.handleErr(err)
				finalizeErr = err
				return
			}
		}
		csvw.Flush()
		if err := csvw.Error(); err != nil {
			m.handleErr(err)
			finalizeErr = err
			return
		}

		// Upload to S3
		input := &s3.PutObjectInput{
			Bucket: &m.bucket,
			Key:    &m.key,
			Body:   bytes.NewReader(buf.Bytes()),
		}
		// Use parentCtx instead of m.ctx for S3 upload to prevent context cancellation issues
		if _, err := m.client.PutObject(m.parentCtx, input); err != nil {
			m.handleErr(err)
			finalizeErr = err
			return
		}

		m.log.Info("manifest finalized and uploaded to S3: %s (records=%d)", m.key, len(m.records))
	})

	return finalizeErr
}

// handleErr invokes the error handler if one is configured.
func (m *manifestImpl) handleErr(err error) {
	if m.errHandler != nil {
		m.errHandler(err)
	}
}

// GenerateManifestKey creates an S3 key for manifest files with timestamp-based organization.
// Files are organized by date folders and include timestamps in the filename.
func GenerateManifestKey(prefix, filename string, t time.Time) string {
	if t.IsZero() {
		t = time.Now().UTC()
	}

	dateFolder := t.Format("2006-01-02")
	if filename == "" {
		filename = "manifest"
	}
	timestamp := t.Format("2006-01-02_15-04-05")
	file := fmt.Sprintf("%s_%s.csv", filename, timestamp)
	// Use path.Join (not filepath.Join) for S3 object keys - always forward slashes
	return path.Join(prefix, dateFolder, file)
}
