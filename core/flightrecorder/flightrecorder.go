package flightrecorder

import "github.com/juju/worker/v4"

// FlightRecorder is the interface for controlling the flight recorder.
type FlightRecorder interface {
	worker.Worker

	// Start starts the flight recorder.
	Start() error

	// Stop stops the flight recorder.
	Stop() error

	// Capture captures a flight recording.
	Capture() error
}
