// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package flightrecorder

import (
	"os"
	"runtime/trace"
	"time"

	"github.com/juju/clock"

	"github.com/juju/juju/internal/errors"
)

const (
	defaultMinAge   = time.Second * 5
	defaultMaxBytes = 1 << 23 // 8 MiB

	defaultFlightRecordingPattern = "flightrecording-"
)

// Recorder is a simple wrapper around trace.Recorder.
type Recorder struct {
	recorder       *trace.FlightRecorder
	clock          clock.Clock
	captureEndTime time.Time
}

// NewRecorder creates a new Recorder.
func NewRecorder(clock clock.Clock) *Recorder {
	return &Recorder{
		recorder: trace.NewFlightRecorder(trace.FlightRecorderConfig{
			MinAge:   defaultMinAge,
			MaxBytes: defaultMaxBytes,
		}),
		clock: clock,
	}
}

// Start starts the flight recorder for at least the supplied duration with the
// recorder only being stopped by a direct call to [Recorder.Stop] or a
// [Recorder.Capture] call that finishes after this duration.
func (w *Recorder) Start(duration time.Duration) error {
	if duration <= 0 {
		w.captureEndTime = w.clock.Now()
	} else {
		w.captureEndTime = w.clock.Now().Add(duration)
	}

	// Don't start if already started, so allow the end time to be changed, but
	// don't fail if it's already running.
	if w.recorder.Enabled() {
		return nil
	}
	return w.recorder.Start()
}

// Stop stops the flight recorder.
func (w *Recorder) Stop() error {
	if !w.recorder.Enabled() {
		return errors.New("recorder is not started")
	}
	w.recorder.Stop()
	return nil
}

// Capture captures a flight recording to the configured path.
func (w *Recorder) Capture(path string) (string, error) {
	if !w.recorder.Enabled() {
		return "", nil
	}

	defer func() {
		if w.clock.Now().After(w.captureEndTime) {
			w.recorder.Stop()
		}
	}()

	f, err := os.CreateTemp(path, defaultFlightRecordingPattern)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	_, err = w.recorder.WriteTo(f)
	return f.Name(), err
}

// Enabled returns whether the recorder is currently enabled.
func (w *Recorder) Enabled() bool {
	return w.recorder.Enabled()
}
