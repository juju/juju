package flightrecorder

import (
	"fmt"

	"github.com/juju/worker/v4"
)

// Kind represents the kind of flight recording.
type Kind string

const (
	// KindRequest indicates a request capture.
	KindRequest Kind = "request"
	// KindError indicates an error capture.
	KindError Kind = "error"
	// Add new kinds here.
	KindAny Kind = "" // special value meaning "any kind"
)

// ParseKind parses a string into a Kind.
func ParseKind(s string) (Kind, error) {
	switch Kind(s) {
	case KindRequest, KindError, KindAny:
		return Kind(s), nil
	default:
		return "", fmt.Errorf("unknown kind %q", s)
	}
}

// IsAllowed returns true if the kind is allowed by the receiver.
func (k Kind) IsAllowed(other Kind) bool {
	if k == KindAny {
		return true
	}
	return k == other
}

// FlightRecorder is the interface for controlling the flight recorder.
type FlightRecorder interface {
	// Start starts the flight recorder.
	Start(kind Kind) error

	// Stop stops the flight recorder.
	Stop() error

	// Capture captures a flight recording.
	Capture(kind Kind) error
}

// FlightRecorderWorker is the interface for a flight recorder worker.
type FlightRecorderWorker interface {
	worker.Worker
	FlightRecorder
}

// NoopRecorder is a no-op implementation of FileRecorder.
type NoopRecorder struct{}

// Start is a no-op.
func (n NoopRecorder) Start(kind Kind) error {
	return nil
}

// Stop is a no-op.
func (n NoopRecorder) Stop() error {
	return nil
}

// Capture is a no-op.
func (n NoopRecorder) Capture(Kind) error {
	return nil
}
