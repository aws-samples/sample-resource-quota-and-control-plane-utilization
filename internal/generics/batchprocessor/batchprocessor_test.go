package batchprocessor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/outofoffice3/aws-samples/geras/internal/logger"
)

// dummy input/output types
type DummyIn struct {
	Val int
}
type DummyOut struct {
	Sq int
}

// helper to record flush calls
type flushRecorder struct {
	calls [][]DummyOut
}

func (r *flushRecorder) Fn(_ context.Context, batch []DummyOut) error {
	// copy batch
	cp := make([]DummyOut, len(batch))
	copy(cp, batch)
	r.calls = append(r.calls, cp)
	return nil
}

// TestEmpty ensures no flush or errors on no input.
func TestEmpty(t *testing.T) {
	rec := &flushRecorder{}
	p := NewGenericBatchProcessor(
		context.Background(),
		10,            // maxBatchSize
		1000,          // maxBatchBytes
		100*time.Hour, // no timer
		func(i DummyIn) (DummyOut, error) {
			return DummyOut{Sq: i.Val * i.Val}, nil
		},
		rec.Fn,
		func(o DummyOut) int { return o.Sq },
		logger.Get(), // any non-nil logger
	)
	p.Wait()
	if len(rec.calls) != 0 {
		t.Fatalf("expected zero flush calls, got %d", len(rec.calls))
	}
}

// TestMapError routes map errors into errCh
func TestMapError(t *testing.T) {
	rec := &flushRecorder{}
	p := NewGenericBatchProcessor(
		context.Background(),
		10, 1000, 100*time.Hour,
		func(i DummyIn) (DummyOut, error) {
			return DummyOut{}, errors.New("bad map")
		},
		rec.Fn,
		func(o DummyOut) int { return 1 },
		logger.Get(),
	)
	p.Add(DummyIn{Val: 3})
	p.Wait()
	if len(rec.calls) != 0 {
		t.Fatalf("expected no flush calls, got %d", len(rec.calls))
	}
}

// TestCountFlush exercises count threshold
func TestCountFlush(t *testing.T) {
	rec := &flushRecorder{}
	p := NewGenericBatchProcessor(
		context.Background(),
		2,   // flush every 2
		100, // large bytes
		100*time.Hour,
		func(i DummyIn) (DummyOut, error) { return DummyOut{Sq: i.Val * i.Val}, nil },
		rec.Fn,
		func(o DummyOut) int { return 1 },
		logger.Get(),
	)
	for i := 1; i <= 5; i++ {
		p.Add(DummyIn{Val: i})
	}
	p.Wait()
	// expect flush at 2,4, then final 1 => 3 calls
	if len(rec.calls) != 3 {
		t.Fatalf("expected 3 flushes, got %d", len(rec.calls))
	}
	// counts of each batch
	wantSizes := []int{2, 2, 1}
	for i, want := range wantSizes {
		if len(rec.calls[i]) != want {
			t.Errorf("batch %d: expected size %d, got %d", i, want, len(rec.calls[i]))
		}
	}
}

// TestByteFlush exercises byte threshold
func TestByteFlush(t *testing.T) {
	rec := &flushRecorder{}
	p := NewGenericBatchProcessor(
		context.Background(),
		1000,
		5, // tiny byte limit
		100*time.Hour,
		func(i DummyIn) (DummyOut, error) { return DummyOut{Sq: i.Val * i.Val}, nil },
		rec.Fn,
		func(o DummyOut) int { return o.Sq }, // sizes 1,4,9,16...
		logger.Get(),
	)
	// Val=1(size1),2(size4) fit => flush before 3(size9)
	for i := 1; i <= 3; i++ {
		p.Add(DummyIn{Val: i})
	}
	p.Wait()
	if len(rec.calls) < 2 {
		t.Fatalf("expected multiple flushes, got %d", len(rec.calls))
	}
}

// TestTimerFlush ensures time‐based flush works
func TestTimerFlush(t *testing.T) {
	rec := &flushRecorder{}
	p := NewGenericBatchProcessor(
		context.Background(),
		1000,
		1000,
		10*time.Millisecond,
		func(i DummyIn) (DummyOut, error) { return DummyOut{Sq: i.Val}, nil },
		rec.Fn,
		func(o DummyOut) int { return 1 },
		logger.Get(),
	)
	p.Add(DummyIn{Val: 7})
	time.Sleep(20 * time.Millisecond)
	p.Wait()
	if len(rec.calls) != 1 {
		t.Fatalf("expected 1 flush by timer, got %d", len(rec.calls))
	}
}

// TestFlushError routes flush errors into errCh
func TestFlushError(t *testing.T) {
	recErr := errors.New("flush failed")
	p := NewGenericBatchProcessor(
		context.Background(),
		10, 1000, 100*time.Hour,
		func(i DummyIn) (DummyOut, error) { return DummyOut{Sq: i.Val}, nil },
		func(_ context.Context, batch []DummyOut) error { return recErr },
		func(o DummyOut) int { return 1 },
		nil, // nil logger must not panic
	)
	p.Add(DummyIn{Val: 5})
	p.Wait()
}

// TestNilLogger ensures no panic if Logger is nil
func TestNilLogger(t *testing.T) {
	rec := &flushRecorder{}
	// Pass nil logger
	p := NewGenericBatchProcessor(
		context.Background(),
		1, 1000, 100*time.Hour,
		func(i DummyIn) (DummyOut, error) { return DummyOut{Sq: i.Val}, nil },
		rec.Fn,
		func(o DummyOut) int { return 1 },
		nil,
	)
	p.Add(DummyIn{Val: 9})
	p.Wait()
	if len(rec.calls) != 1 {
		t.Fatalf("expected 1 flush, got %d", len(rec.calls))
	}
}
