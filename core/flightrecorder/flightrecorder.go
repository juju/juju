package flightrecorder

// FlightRecorder is the interface for controlling the flight recorder.
type FlightRecorder interface {
	// Start starts the flight recorder.
	Start() error

	// Stop stops the flight recorder.
	Stop() error

	// Capture captures a flight recording.
	Capture() error
}
