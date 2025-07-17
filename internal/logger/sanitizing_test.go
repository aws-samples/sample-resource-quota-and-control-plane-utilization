package logger

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockLogger for testing SanitizingLogger
type MockLogger struct {
	mock.Mock
}

func (m *MockLogger) Debug(msg string, args ...any) {
	m.Called(msg, args)
}

func (m *MockLogger) Info(msg string, args ...any) {
	m.Called(msg, args)
}

func (m *MockLogger) Warn(msg string, args ...any) {
	m.Called(msg, args)
}

func (m *MockLogger) Error(msg string, args ...any) {
	m.Called(msg, args)
}

func TestNewSanitizingLogger(t *testing.T) {
	mockLogger := &MockLogger{}
	sanitizingLogger := NewSanitizingLogger(mockLogger)
	
	assert.NotNil(t, sanitizingLogger)
	assert.IsType(t, &SanitizingLogger{}, sanitizingLogger)
}

func TestSanitizingLogger_SanitizeArgs(t *testing.T) {
	mockLogger := &MockLogger{}
	sanitizingLogger := &SanitizingLogger{underlying: mockLogger}
	
	tests := []struct {
		name     string
		args     []any
		expected []any
	}{
		{
			name:     "empty args",
			args:     []any{},
			expected: []any{},
		},
		{
			name:     "no string args",
			args:     []any{1, 2.0, true, nil},
			expected: []any{1, 2.0, true, nil},
		},
		{
			name:     "string args",
			args:     []any{"normal", "with\nnewline", "with\ttab"},
			expected: []any{"normal", "with_newline", "with_tab"},
		},
		{
			name:     "mixed args",
			args:     []any{1, "normal", 2.0, "with\nnewline", true},
			expected: []any{1, "normal", 2.0, "with_newline", true},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizingLogger.sanitizeArgs(tt.args...)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSanitizingLogger_Debug(t *testing.T) {
	mockLogger := &MockLogger{}
	sanitizingLogger := &SanitizingLogger{underlying: mockLogger}
	
	// Setup expectations
	mockLogger.On("Debug", "test message", []any{"sanitized_string"}).Return()
	
	// Call the method
	sanitizingLogger.Debug("test message", "sanitized\nstring")
	
	// Verify expectations
	mockLogger.AssertExpectations(t)
}

func TestSanitizingLogger_Info(t *testing.T) {
	mockLogger := &MockLogger{}
	sanitizingLogger := &SanitizingLogger{underlying: mockLogger}
	
	// Setup expectations
	mockLogger.On("Info", "test message", []any{"sanitized_string"}).Return()
	
	// Call the method
	sanitizingLogger.Info("test message", "sanitized\nstring")
	
	// Verify expectations
	mockLogger.AssertExpectations(t)
}

func TestSanitizingLogger_Warn(t *testing.T) {
	mockLogger := &MockLogger{}
	sanitizingLogger := &SanitizingLogger{underlying: mockLogger}
	
	// Setup expectations
	mockLogger.On("Warn", "test message", []any{"sanitized_string"}).Return()
	
	// Call the method
	sanitizingLogger.Warn("test message", "sanitized\nstring")
	
	// Verify expectations
	mockLogger.AssertExpectations(t)
}

func TestSanitizingLogger_Error(t *testing.T) {
	mockLogger := &MockLogger{}
	sanitizingLogger := &SanitizingLogger{underlying: mockLogger}
	
	// Setup expectations
	mockLogger.On("Error", "test message", []any{"sanitized_string"}).Return()
	
	// Call the method
	sanitizingLogger.Error("test message", "sanitized\nstring")
	
	// Verify expectations
	mockLogger.AssertExpectations(t)
}

func TestSanitizingLogger_MultipleArgs(t *testing.T) {
	mockLogger := &MockLogger{}
	sanitizingLogger := &SanitizingLogger{underlying: mockLogger}
	
	// Setup expectations with multiple arguments
	mockLogger.On("Info", "test %s %d", []any{"sanitized_string", 42}).Return()
	
	// Call the method
	sanitizingLogger.Info("test %s %d", "sanitized\nstring", 42)
	
	// Verify expectations
	mockLogger.AssertExpectations(t)
}

func TestSanitizingLogger_NoArgs(t *testing.T) {
	mockLogger := &MockLogger{}
	sanitizingLogger := &SanitizingLogger{underlying: mockLogger}
	
	// Setup expectations with nil arguments slice
	mockLogger.On("Info", "test message", []any(nil)).Return()
	
	// Call the method
	sanitizingLogger.Info("test message")
	
	// Verify expectations
	mockLogger.AssertExpectations(t)
}

func TestSanitizingLogger_ControlCharacters(t *testing.T) {
	mockLogger := &MockLogger{}
	sanitizingLogger := &SanitizingLogger{underlying: mockLogger}
	
	// String with various control characters
	inputString := "test\nwith\tnewline\rand\x00null\x1Fchars"
	expectedString := "test_with_newline_and_null_chars"
	
	// Setup expectations
	mockLogger.On("Info", "message", []any{expectedString}).Return()
	
	// Call the method
	sanitizingLogger.Info("message", inputString)
	
	// Verify expectations
	mockLogger.AssertExpectations(t)
}