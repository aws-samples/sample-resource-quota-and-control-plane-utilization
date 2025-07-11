package logger

import "github.com/outofoffice3/aws-samples/geras/internal/utils"

// SanitizingLogger wraps an existing Logger and automatically sanitizes
// all string arguments to prevent log injection attacks.
type SanitizingLogger struct {
	underlying Logger
}

// NewSanitizingLogger creates a new sanitizing wrapper around the provided logger.
func NewSanitizingLogger(underlying Logger) Logger {
	return &SanitizingLogger{underlying: underlying}
}

// sanitizeArgs processes all arguments and sanitizes any string values.
func (s *SanitizingLogger) sanitizeArgs(args ...any) []any {
	if len(args) == 0 {
		return args
	}
	
	sanitized := make([]any, len(args))
	for i, arg := range args {
		if str, ok := arg.(string); ok {
			sanitized[i] = utils.SanitizeLogString(str)
		} else {
			sanitized[i] = arg
		}
	}
	return sanitized
}

func (s *SanitizingLogger) Debug(msg string, args ...any) {
	s.underlying.Debug(msg, s.sanitizeArgs(args...)...)
}

func (s *SanitizingLogger) Info(msg string, args ...any) {
	s.underlying.Info(msg, s.sanitizeArgs(args...)...)
}

func (s *SanitizingLogger) Warn(msg string, args ...any) {
	s.underlying.Warn(msg, s.sanitizeArgs(args...)...)
}

func (s *SanitizingLogger) Error(msg string, args ...any) {
	s.underlying.Error(msg, s.sanitizeArgs(args...)...)
}