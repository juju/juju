// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package flightrecorder

import (
	"os"
	"runtime/trace"
	"time"
)

// Recorder is a simple wrapper around trace.Recorder.
type Recorder struct {
	recorder *trace.FlightRecorder
}

// NewRecorder creates a new Recorder.
func NewRecorder() *Recorder {
	return &Recorder{
		recorder: trace.NewFlightRecorder(trace.FlightRecorderConfig{
			MinAge:   time.Second,
			MaxBytes: 1 << 20, // 1 MiB
		}),
	}
}

// Start starts the flight recorder.
func (w *Recorder) Start() error {
	return w.recorder.Start()
}

// Stop stops the flight recorder.
func (w *Recorder) Stop() error {
	w.recorder.Stop()
	return nil
}

// Capture captures a flight recording to the configured path.
func (w *Recorder) Capture(path string) (string, error) {
	if !w.recorder.Enabled() {
		return "", nil
	}

	defer w.recorder.Stop()

	f, err := os.CreateTemp(path, "flightrecording-")
	if err != nil {
		return "", err
	}
	defer f.Close()

	_, err = w.recorder.WriteTo(f)
	return f.Name(), err
}
