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
			debugLogger: log.New(writer, "DEBUG: ", log.LstdFlags|log.Lmsgprefix),
			infoLogger:  log.New(writer, "INFO:  ", log.LstdFlags|log.Lmsgprefix),
			warnLogger:  log.New(writer, "WARN:  ", log.LstdFlags|log.Lmsgprefix),
			errorLogger: log.New(writer, "ERROR: ", log.LstdFlags|log.Lmsgprefix),
		}
	})
}

// Get returns the initialized Logger instance wrapped with auto-sanitization.
// If Init has not been called, it initializes with default INFO level writing to stdout.
func Get() Logger {
	if defaultLogger == nil {
		Init(level, os.Stdout)
	}
	return NewSanitizingLogger(defaultLogger)
}

// stdLogger wraps the standard log.Logger and respects the configured level.
// It implements Logger.
type stdLogger struct {
	debugLogger *log.Logger
	infoLogger  *log.Logger
	warnLogger  *log.Logger
	errorLogger *log.Logger
}

func (l *stdLogger) Debug(msg string, args ...any) {
	if level <= DEBUG {
		l.debugLogger.Printf(msg, args...)
	}
}

func (l *stdLogger) Info(msg string, args ...any) {
	if level <= INFO {
		l.infoLogger.Printf(msg, args...)
	}
}

func (l *stdLogger) Warn(msg string, args ...any) {
	if level <= WARN {
		l.warnLogger.Printf(msg, args...)
	}
}

func (l *stdLogger) Error(msg string, args ...any) {
	if level <= ERROR {
		l.errorLogger.Printf(msg, args...)
	}
}
