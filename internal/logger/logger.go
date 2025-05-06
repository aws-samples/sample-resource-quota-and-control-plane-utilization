package logger

import (
	"io"
	"log"
	"os"
	"sync"
)

// LogLevel defines severity levels for logging.
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
)

// Logger is the interface for our logging abstraction.
type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

var (
	defaultLogger Logger
	once          sync.Once
	level         LogLevel = INFO
)

// Init configures the package-level logger with the desired log level
// and output destination. It will only apply once; subsequent calls
// have no effect.
func Init(lvl LogLevel, out io.Writer) {
	once.Do(func() {
		level = lvl
		writer := out
		if writer == nil {
			writer = os.Stdout
		}
		defaultLogger = &stdLogger{
			logger: log.New(writer, "", log.LstdFlags|log.Lmsgprefix),
		}
	})
}

// Get returns the initialized Logger instance. If Init has not been called,
// it initializes with default INFO level writing to stdout.
func Get() Logger {
	if defaultLogger == nil {
		Init(level, os.Stdout)
	}
	return defaultLogger
}

// stdLogger wraps the standard log.Logger and respects the configured level.
// It implements Logger.
type stdLogger struct {
	logger *log.Logger
}

func (l *stdLogger) Debug(msg string, args ...any) {
	if level <= DEBUG {
		l.logger.SetPrefix("DEBUG: ")
		l.logger.Printf(msg, args...)
	}
}

func (l *stdLogger) Info(msg string, args ...any) {
	if level <= INFO {
		l.logger.SetPrefix("INFO:  ")
		l.logger.Printf(msg, args...)
	}
}

func (l *stdLogger) Warn(msg string, args ...any) {
	if level <= WARN {
		l.logger.SetPrefix("WARN:  ")
		l.logger.Printf(msg, args...)
	}
}

func (l *stdLogger) Error(msg string, args ...any) {
	if level <= ERROR {
		l.logger.SetPrefix("ERROR: ")
		l.logger.Printf(msg, args...)
	}
}
