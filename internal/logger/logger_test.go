// internal/logger/logger_test.go
package logger

import (
	"bytes"
	"io"
	"sync"
	"testing"
)

// resetState reinitializes package‐level vars so each test starts clean.
func resetState() {
	once = sync.Once{}
	defaultLogger = nil
	level = INFO
}

func TestInitAndGet_Defaults(t *testing.T) {
	resetState()
	// Get before Init should auto‐Init at INFO level to stdout.
	l := Get()
	if l == nil {
		t.Fatal("Get returned nil logger")
	}
	// Should not panic even if Debug is suppressed
	l.Debug("debug %v", 1)
	l.Info("info %v", 2)
}

func TestInit_CustomWriterAndLevel(t *testing.T) {
	resetState()
	var buf bytes.Buffer

	// First Init applies DEBUG level to buf
	Init(DEBUG, &buf)
	// Second Init is a no-op
	Init(ERROR, io.Discard)

	l := Get()
	l.Debug("d %d", 10)
	l.Info("i %d", 20)
	l.Warn("w %d", 30)
	l.Error("e %d", 40)

	out := buf.String()
	if count := bytes.Count([]byte(out), []byte("DEBUG:")); count != 1 {
		t.Errorf("expected 1 DEBUG, got %d", count)
	}
	if count := bytes.Count([]byte(out), []byte("INFO:")); count != 1 {
		t.Errorf("expected 1 INFO, got %d", count)
	}
	if count := bytes.Count([]byte(out), []byte("WARN:")); count != 1 {
		t.Errorf("expected 1 WARN, got %d", count)
	}
	if count := bytes.Count([]byte(out), []byte("ERROR:")); count != 1 {
		t.Errorf("expected 1 ERROR, got %d", count)
	}
}

func TestLogLevelFiltering(t *testing.T) {
	resetState()
	var buf bytes.Buffer

	// Set level to WARN
	Init(WARN, &buf)
	l := Get()

	// DEBUG and INFO suppressed
	l.Debug("skip debug")
	l.Info("skip info")
	l.Warn("show warn")
	l.Error("show error")

	out := buf.String()
	if bytes.Contains([]byte(out), []byte("DEBUG:")) {
		t.Error("DEBUG should be suppressed at WARN level")
	}
	if bytes.Contains([]byte(out), []byte("INFO:")) {
		t.Error("INFO should be suppressed at WARN level")
	}
	if !bytes.Contains([]byte(out), []byte("WARN:")) {
		t.Error("WARN should appear at WARN level")
	}
	if !bytes.Contains([]byte(out), []byte("ERROR:")) {
		t.Error("ERROR should appear at WARN level")
	}
}

func TestErrorAlwaysLogs(t *testing.T) {
	resetState()
	var buf bytes.Buffer

	// Set level to ERROR
	Init(ERROR, &buf)
	l := Get()

	l.Debug("d")
	l.Info("i")
	l.Warn("w")
	l.Error("e%v", 5)

	out := buf.String()
	if bytes.Contains([]byte(out), []byte("DEBUG:")) {
		t.Error("DEBUG should be suppressed at ERROR level")
	}
	if bytes.Contains([]byte(out), []byte("INFO:")) {
		t.Error("INFO should be suppressed at ERROR level")
	}
	if bytes.Contains([]byte(out), []byte("WARN:")) {
		t.Error("WARN should be suppressed at ERROR level")
	}
	if count := bytes.Count([]byte(out), []byte("ERROR:")); count != 1 {
		t.Errorf("expected 1 ERROR, got %d", count)
	}
}
