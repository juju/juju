package flightrecorder

import "github.com/juju/worker/v4"

// FlightRecorder is the interface for controlling the flight recorder.
type FlightRecorder interface {
	// Start starts the flight recorder.
	Start() error

	// Stop stops the flight recorder.
	Stop() error

	// Capture captures a flight recording.
	Capture() error
}

// FlightRecorderWorker is the interface for a flight recorder worker.
type FlightRecorderWorker interface {
	worker.Worker
	FlightRecorder
}

// NoopRecorder is a no-op implementation of FileRecorder.
type NoopRecorder struct{}

// Start is a no-op.
func (n NoopRecorder) Start() error {
	return nil
}

// Stop is a no-op.
func (n NoopRecorder) Stop() error {
	return nil
}

// Capture is a no-op.
func (n NoopRecorder) Capture() error {
	return nil
}
