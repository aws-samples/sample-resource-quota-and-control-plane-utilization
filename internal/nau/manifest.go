package nau

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"path"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/s3client"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
)

// ErrHeaderNotWritten is returned when WriteRecord is called before WriteHeader.
var ErrHeaderNotWritten = errors.New("manifest header not written")

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
	// WriteHeader sets the CSV header columns. Must be called before WriteRecord.
	WriteHeader(columns []string) error
	// WriteRecord buffers a CSVRecord for later CSV generation.
	WriteRecord(rec CSVRecord) error
	// Finalize generates the complete CSV and uploads it to S3.
	Finalize() error
}

// manifestImpl implements the Manifest interface with S3 upload capability.
type manifestImpl struct {
	ctx        context.Context   // Context for S3 operations
	bucket     string            // S3 bucket name
	key        string            // S3 object key
	client     s3client.S3Client // S3 client for uploads
	header     []string          // CSV header columns
	records    [][]string        // Buffered CSV records
	errHandler ErrorHandler      // Error handling callback
	log        logger.Logger     // Logger instance
	finalized  bool              // Tracks finalization state
}

// NewManifest creates a new Manifest instance for CSV generation and S3 upload.
// The manifest will buffer records and upload them to the specified S3 location on finalization.
func NewManifest(ctx context.Context, bucket, key string, client s3client.S3Client, errHandler ErrorHandler, log logger.Logger) Manifest {
	log.Debug("creating new manifest for S3 upload: bucket=%s key=%s", bucket, key)
	metadata := ResourceMetadata{}
	return &manifestImpl{
		ctx:        ctx,
		bucket:     bucket,
		key:        key,
		client:     client,
		errHandler: errHandler,
		header:     metadata.Header(),
		log:        log,
	}
}

// WriteHeader sets the CSV header columns. Subsequent calls are ignored.
func (m *manifestImpl) WriteHeader(columns []string) error {
	if m.header != nil {
		return nil
	}
	m.header = append([]string(nil), columns...)
	m.log.Info("manifest header set: %v", m.header)
	return nil
}

// WriteRecord adds a CSV record to the buffer for later processing.
// Returns ErrHeaderNotWritten if WriteHeader hasn't been called first.
func (m *manifestImpl) WriteRecord(rec CSVRecord) error {
	if m.header == nil {
		m.log.Warn("WriteRecord called before WriteHeader")
		return ErrHeaderNotWritten
	}
	vals := append([]string(nil), rec.Values()...)
	m.records = append(m.records, vals)
	m.log.Debug("buffered record: %v", vals)
	return nil
}

// Finalize generates the complete CSV from buffered records and uploads to S3.
// This operation is idempotent - subsequent calls after success are no-ops.
func (m *manifestImpl) Finalize() error {
	if m.finalized {
		return nil
	}
	if m.header == nil {
		return ErrHeaderNotWritten
	}

	// Build CSV in memory
	buf := &bytes.Buffer{}
	csvw := csv.NewWriter(buf)
	if err := csvw.Write(m.header); err != nil {
		m.handleErr(err)
		return err
	}

	m.log.Info("record count : %d", len(m.records))
	for _, row := range m.records {
		if err := csvw.Write(row); err != nil {
			m.handleErr(err)
			return err
		}
	}
	csvw.Flush()
	if err := csvw.Error(); err != nil {
		m.handleErr(err)
		return err
	}

	// Upload to S3
	input := &s3.PutObjectInput{
		Bucket: &m.bucket,
		Key:    &m.key,
		Body:   bytes.NewReader(buf.Bytes()),
	}
	if _, err := m.client.PutObject(m.ctx, input); err != nil {
		m.handleErr(err)
		return err
	}

	m.finalized = true
	m.log.Info("manifest finalized and uploaded to S3: %s (records=%d)", m.key, len(m.records))
	return nil
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
