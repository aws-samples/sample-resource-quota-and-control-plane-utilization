package logger

// noopLogger implements Logger but does nothing.
type NoopLogger struct{}

func (n *NoopLogger) Debug(_ string, _ ...any) {}
func (n *NoopLogger) Info(_ string, _ ...any)  {}
func (n *NoopLogger) Warn(_ string, _ ...any)  {}
func (n *NoopLogger) Error(_ string, _ ...any) {}
