// Package emf provides functionality for creating and sending Embedded Metric Format (EMF)
// documents to CloudWatch Logs for automatic metric extraction and visualization.
package emf

import (
	"github.com/outofoffice3/aws-samples/geras/internal/emf/builder"
	"github.com/outofoffice3/aws-samples/geras/internal/emf/flusher"
)

type EMFInput = builder.EMFInput
type EMFRecord = builder.EMFRecord

var Build = builder.Build

type EMFFlusher = flusher.EMFFlusher

var NewEMFFlusher = flusher.NewEMFFlusher
