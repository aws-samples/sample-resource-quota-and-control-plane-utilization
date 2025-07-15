package nau

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrorVariables(t *testing.T) {
	tests := []struct {
		name string
		err  error
		msg  string
	}{
		{"errUnsupportedEni", errUnsupportedEni, "unknown eni type"},
		{"errNonAttachedEni", errNonAttachedEni, "non attached eni"},
		{"ErrHeaderNotWritten", ErrHeaderNotWritten, "manifest header not written"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Error(t, tt.err, "Should be an error")
			assert.Equal(t, tt.msg, tt.err.Error(), "Error message should match")
		})
	}
}

func TestErrorsIs(t *testing.T) {
	// Test that errors.Is works correctly with our error variables
	testErr := errors.New("some wrapped error")
	wrappedUnsupported := errors.Join(errUnsupportedEni, testErr)
	wrappedNonAttached := errors.Join(errNonAttachedEni, testErr)

	assert.True(t, errors.Is(wrappedUnsupported, errUnsupportedEni), "Should detect wrapped unsupported ENI error")
	assert.True(t, errors.Is(wrappedNonAttached, errNonAttachedEni), "Should detect wrapped non-attached ENI error")
	assert.False(t, errors.Is(wrappedUnsupported, errNonAttachedEni), "Should not cross-match error types")
}