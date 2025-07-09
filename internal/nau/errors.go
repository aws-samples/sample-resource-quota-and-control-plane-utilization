package nau

import (
	"errors"
)

// Error variables for NAU calculation operations.
var (
	// errUnsupportedEni indicates an ENI type that is not supported for NAU calculation.
	errUnsupportedEni error = errors.New("unknown eni type")
	// errNonAttachedEni indicates an ENI that is not attached to any resource.
	errNonAttachedEni error = errors.New("non attached eni")
)
