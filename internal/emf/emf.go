// Package emf provides a unified interface for creating and sending Embedded Metric Format (EMF)
// documents to CloudWatch Logs. It re-exports key types and functions from subpackages
// for convenient access to EMF building and flushing functionality.
package emf

import (
	"github.com/outofoffice3/aws-samples/geras/internal/emf/builder"
	"github.com/outofoffice3/aws-samples/geras/internal/emf/flusher"
)

// EMFInput is an alias for builder.EMFInput containing metric parameters.
type EMFInput = builder.EMFInput

// EMFRecord is an alias for builder.EMFRecord containing the complete EMF document.
type EMFRecord = builder.EMFRecord

// Build is an alias for builder.Build function that creates EMF documents.
var Build = builder.Build

// EMFFlusher is an alias for flusher.EMFFlusher interface for sending EMF records.
type EMFFlusher = flusher.EMFFlusher

// NewEMFFlusher is an alias for flusher.NewEMFFlusher constructor function.
var NewEMFFlusher = flusher.NewEMFFlusher
