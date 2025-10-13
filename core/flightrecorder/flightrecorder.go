// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package flightrecorder

import (
	"fmt"
	"time"

	"github.com/juju/worker/v4"
)

// Kind represents the kind of flight recording.
type Kind string

const (
	// KindRequest indicates a request capture.
	KindRequest Kind = "request"
	// KindError indicates an error capture.
	KindError Kind = "error"
	// KindAll indicates all captures.
	KindAll Kind = "all"
)

// ParseKind parses a string into a Kind.
func ParseKind(s string) (Kind, error) {
	switch s {
	case "request", "error":
		return Kind(s), nil
	case "all", "":
		return KindAll, nil
	default:
		return "", fmt.Errorf("unknown kind %q", s)
	}
}

// IsAllowed returns true if the kind is allowed by the receiver.
func (k Kind) IsAllowed(other Kind) bool {
	if k == KindAll {
		return true
	}
	return k == other
}

// FlightRecorder is the interface for controlling the flight recorder.
type FlightRecorder interface {
	// Start starts the flight recorder.
	Start(kind Kind, duration time.Duration) error

	// Stop stops the flight recorder.
	Stop() error

	// Capture captures a flight recording.
	Capture(kind Kind) error

	// Enabled returns whether the recorder is currently recording.
	Enabled() bool
}

// FlightRecorderWorker is the interface for a flight recorder worker.
type FlightRecorderWorker interface {
	worker.Worker
	FlightRecorder
}

// NoopRecorder is a no-op implementation of FileRecorder.
type NoopRecorder struct{}

// Start is a no-op.
func (n NoopRecorder) Start(kind Kind, duration time.Duration) error {
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

// Enabled always returns false.
func (n NoopRecorder) Enabled() bool {
	return false
}
