package logger

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNoopLogger_Methods(t *testing.T) {
	logger := &NoopLogger{}
	
	// Test that all methods can be called without panicking
	t.Run("Debug method", func(t *testing.T) {
		assert.NotPanics(t, func() {
			logger.Debug("test message")
			logger.Debug("test message with args: %v, %v", 1, "string")
			logger.Debug("test with nil arg", nil)
		})
	})
	
	t.Run("Info method", func(t *testing.T) {
		assert.NotPanics(t, func() {
			logger.Info("test message")
			logger.Info("test message with args: %v, %v", 1, "string")
			logger.Info("test with nil arg", nil)
		})
	})
	
	t.Run("Warn method", func(t *testing.T) {
		assert.NotPanics(t, func() {
			logger.Warn("test message")
			logger.Warn("test message with args: %v, %v", 1, "string")
			logger.Warn("test with nil arg", nil)
		})
	})
	
	t.Run("Error method", func(t *testing.T) {
		assert.NotPanics(t, func() {
			logger.Error("test message")
			logger.Error("test message with args: %v, %v", 1, "string")
			logger.Error("test with nil arg", nil)
		})
	})
}

func TestNoopLogger_ImplementsInterface(t *testing.T) {
	// Verify that NoopLogger implements the Logger interface
	var logger Logger = &NoopLogger{}
	assert.NotNil(t, logger)
	
	// Call methods through the interface
	assert.NotPanics(t, func() {
		logger.Debug("test")
		logger.Info("test")
		logger.Warn("test")
		logger.Error("test")
	})
}