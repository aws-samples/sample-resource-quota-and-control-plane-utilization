package batchprocessor

import (
	"context"
	"sync"
	"time"

	applogger "github.com/outofoffice3/aws-samples/geras/internal/logger"
)

// GenericBatchProcessor buffers converted items O until one of:
//   - MaxBatchSize items,
//   - MaxBatchBytes total size,
//   - FlushInterval elapses,
//
// at which point FlushFunc is invoked with the O-typed batch.
type GenericBatchProcessor[I any, O any] struct {
	MaxBatchSize  int
	MaxBatchBytes int
	FlushInterval time.Duration

	MapFunc    func(I) (O, error)
	ItemSizer  func(O) int
	FlushFunc  func(ctx context.Context, batch []O) error
	AfterFlush func(flushed []O)

	in     chan I
	wg     *sync.WaitGroup
	Logger applogger.Logger
}

type GenericOption[I any, O any] func(*GenericBatchProcessor[I, O])

func WithAfterFlush[I any, O any](f func([]O)) GenericOption[I, O] {
	return func(p *GenericBatchProcessor[I, O]) {
		p.AfterFlush = f
	}
}

// NewGenericBatchProcessor creates and starts a GenericBatchProcessor.
// It returns immediately; processing happens in the background.
func NewGenericBatchProcessor[I any, O any](
	ctx context.Context,
	maxBatchSize, maxBatchBytes int,
	flushInterval time.Duration,
	mapFunc func(I) (O, error),
	flushFunc func(ctx context.Context, batch []O) error,
	itemSizer func(O) int,
	logger applogger.Logger,
	opts ...GenericOption[I, O],
) *GenericBatchProcessor[I, O] {
	if logger == nil {
		logger = &applogger.NoopLogger{}
	}
	p := &GenericBatchProcessor[I, O]{
		MaxBatchSize:  maxBatchSize,
		MaxBatchBytes: maxBatchBytes,
		FlushInterval: flushInterval,
		MapFunc:       mapFunc,
		ItemSizer:     itemSizer,
		FlushFunc:     flushFunc,
		in:            make(chan I, 100),
		wg:            &sync.WaitGroup{},
		Logger:        logger,
	}

	// apply options
	for _, opt := range opts {
		opt(p)
	}

	p.wg.Add(1)
	go p.start(ctx)
	// log the type of the batch
	logger.Debug("batch processor generic successfully started")
	return p
}

func (p *GenericBatchProcessor[I, O]) start(ctx context.Context) {
	defer p.wg.Done()

	var (
		batch        []O
		currentBytes int
	)

	// Set up ticker for time-based flush
	var tick <-chan time.Time
	if p.FlushInterval > 0 {
		t := time.NewTicker(p.FlushInterval)
		defer t.Stop()
		tick = t.C
	}

	// flush sends the batch and resets state
	flush := func() {
		if len(batch) == 0 {
			p.Logger.Debug("nothing to flush")
			return
		}
		err := p.FlushFunc(ctx, batch)
		if err != nil {
			HandleError(err, p.Logger)
		}
		if p.AfterFlush != nil {
			p.AfterFlush(batch)
			p.Logger.Debug("after flush hook called")
		}
		batch = make([]O, 0, p.MaxBatchSize)
		currentBytes = 0
	}

	for {
		select {
		case <-ctx.Done():
			p.Logger.Info("context done. flushing buffer")
			flush()
			return

		case item, ok := <-p.in:
			if !ok {
				p.Logger.Info("input channel closed. flushing buffer")
				flush()
				return
			}

			// 1) map input → output
			out, err := p.MapFunc(item)
			if err != nil {
				HandleError(err, p.Logger)
				continue
			}

			// 2) pre-add byte-limit check
			itemSize := p.ItemSizer(out)
			if p.MaxBatchBytes > 0 && currentBytes+itemSize > p.MaxBatchBytes {
				p.Logger.Info("max batch bytes reached. flushing buffer")
				flush()
			}

			// 3) accumulate
			batch = append(batch, out)
			currentBytes += itemSize

			// 4) count-based flush
			if p.MaxBatchSize > 0 && len(batch) >= p.MaxBatchSize {
				p.Logger.Info("max batch size reached. flushing buffer")
				flush()
			}

		case <-tick:
			p.Logger.Info("time interval reached. flushing buffer")
			flush()
		}
	}
}

// Add enqueues one item of type I.
func (p *GenericBatchProcessor[I, O]) Add(item I) {
	p.in <- item
	p.Logger.Debug("item added to buffer %#v", item)
}

// Wait closes the input channel and blocks until all background work
// (including the final flush) has completed.
func (p *GenericBatchProcessor[I, O]) Wait() {
	close(p.in)
	p.Logger.Info("batch processor generic input channel closed")
	p.wg.Wait()
	p.Logger.Info("batch processor generic wait done")
}

// GetInputChannel
func (p *GenericBatchProcessor[I, O]) GetInputChannel() chan I {
	return p.in
}

// Handle Error
func HandleError(err error, logger applogger.Logger) {
	logger.Error("error: %v", err)
}
