// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package flightrecorder

import (
	"context"
	"errors"
	"time"

	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/flightrecorder"
	"github.com/juju/juju/core/logger"
)

// FileRecorder defines the interface for a flight recorder that can
// start, stop and capture recordings to a file.
type FileRecorder interface {
	// Start starts the flight recorder.
	Start(time.Duration) error

	// Stop stops the flight recorder.
	Stop() error

	// Capture captures a flight recording to the given path.
	Capture(path string) (string, error)

	// Enabled returns whether the recorder is currently recording.
	Enabled() bool
}

type requestType int

const (
	requestTypeStart requestType = iota
	requestTypeStop
	requestTypeEnabled
)

type request struct {
	Type     requestType
	Kind     flightrecorder.Kind
	Duration time.Duration
	Result   chan response
}

type captureRequest struct {
	Kind   flightrecorder.Kind
	Result chan response
}

type response struct {
	Error   error
	Enabled bool
}

type report struct {
	Enabled bool
	Kind    flightrecorder.Kind
}

// FlightRecorder is the flight recorder worker.
//
// There can be only one flight recorder, and we need to put it inside of the
// engine, so it also needs to be a worker.
//
// The worker is also sequenced into a serialized request loop, so that
// it delivers predictable results when faced with concurrent requests.
type FlightRecorder struct {
	tomb tomb.Tomb

	path     string
	recorder FileRecorder

	currentKind    flightrecorder.Kind
	requests       chan request
	captureRequest chan captureRequest

	reports chan chan report

	logger logger.Logger
}

// New creates a new flight recorder worker.
func New(recorder FileRecorder, path string, logger logger.Logger) *FlightRecorder {
	w := &FlightRecorder{
		recorder:    recorder,
		path:        path,
		currentKind: flightrecorder.KindAll,

		requests: make(chan request),
		reports:  make(chan chan report),

		// Only allow one capture request at a time, if there are multiple ones
		// in flight, ignore it and return an error to allow the caller
		// to decide what to do.
		captureRequest: make(chan captureRequest, 1),

		logger: logger,
	}

	w.tomb.Go(w.loop)

	return w
}

// Kill stops the worker.
func (w *FlightRecorder) Kill() {
	w.tomb.Kill(nil)
}

// Wait waits for the worker to stop.
func (w *FlightRecorder) Wait() error {
	return w.tomb.Wait()
}

// Start starts the flight recorder for at least the supplied duration with the
// recorder only being stopped by a direct call to [Recorder.Stop] or a
// [Recorder.Capture] call that finishes after this duration.
func (w *FlightRecorder) Start(kind flightrecorder.Kind, duration time.Duration) error {
	request := request{
		Type:     requestTypeStart,
		Kind:     kind,
		Duration: duration,
		Result:   make(chan response, 1),
	}

	select {
	case <-w.tomb.Dying():
		return errors.New("worker is stopping")
	case w.requests <- request:
	}

	select {
	case <-w.tomb.Dying():
		return errors.New("worker is stopping")
	case response := <-request.Result:
		return response.Error
	}
}

// Stop stops the flight recorder.
func (w *FlightRecorder) Stop() error {
	result := make(chan response, 1)
	req := request{
		Type:   requestTypeStop,
		Result: result,
	}

	select {
	case <-w.tomb.Dying():
		return errors.New("worker is stopping")
	case w.requests <- req:
	}

	select {
	case <-w.tomb.Dying():
		return errors.New("worker is stopping")
	case response := <-result:
		return response.Error
	}
}

// Capture instructs the flight recorder to capture a recording of the
// specified kind. If the recorder is not currently recording, this is a no-op.
// If the capture is already in progress, an error is returned, to prevent
// piling up multiple requests.
func (w *FlightRecorder) Capture(kind flightrecorder.Kind) error {
	result := make(chan response, 1)
	req := captureRequest{
		Kind:   kind,
		Result: result,
	}

	select {
	case <-w.tomb.Dying():
		return errors.New("worker is stopping")
	case w.captureRequest <- req:
	default:
		// If there is already a capture request in flight, return an error,
		// so we don't pile up multiple requests.
		return errors.New("flight recorder capture already in progress")
	}

	select {
	case <-w.tomb.Dying():
		return errors.New("worker is stopping")
	case response := <-result:
		return response.Error
	}
}

// Enabled returns whether the recorder is currently recording.
func (w *FlightRecorder) Enabled() bool {
	result := make(chan response, 1)
	req := request{
		Type:   requestTypeEnabled,
		Result: result,
	}

	select {
	case <-w.tomb.Dying():
		return false
	case w.requests <- req:
	}

	select {
	case <-w.tomb.Dying():
		return false
	case response := <-result:
		return response.Enabled
	}
}

// Report returns a map of internal state for introspection.
func (w *FlightRecorder) Report() map[string]any {
	ch := make(chan report, 1)
	select {
	case <-w.tomb.Dying():
		return map[string]any{}
	case w.reports <- ch:
	}

	select {
	case <-w.tomb.Dying():
		return map[string]any{}
	case r := <-ch:
		return map[string]any{
			"enabled": r.Enabled,
			"kind":    r.Kind,
		}
	}
}

func (w *FlightRecorder) loop() error {
	ctx := w.tomb.Context(context.Background())

	defer func() { _ = w.recorder.Stop() }()

	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying

		case req := <-w.requests:
			var (
				err     error
				enabled bool
			)
			switch req.Type {
			case requestTypeStart:
				err = w.startRecording(ctx, req.Kind, req.Duration)
			case requestTypeStop:
				err = w.stopRecording(ctx)
			case requestTypeEnabled:
				enabled = w.recorder.Enabled()
			default:
				err = errors.New("unknown request type")
			}

			select {
			case <-w.tomb.Dying():
				return tomb.ErrDying
			case req.Result <- response{Error: err, Enabled: enabled}:
			}

		case req := <-w.captureRequest:
			err := w.captureRecording(ctx, req.Kind)

			select {
			case <-w.tomb.Dying():
				return tomb.ErrDying
			case req.Result <- response{Error: err}:
			}

		case res := <-w.reports:
			select {
			case <-w.tomb.Dying():
				return tomb.ErrDying
			case res <- report{
				Enabled: w.recorder.Enabled(),
				Kind:    w.currentKind,
			}:
			}
		}
	}
}

func (w *FlightRecorder) startRecording(ctx context.Context, kind flightrecorder.Kind, duration time.Duration) error {
	w.logger.Debugf(ctx, "starting flight recording for kind %q", kind)

	w.currentKind = kind

	if duration < 0 {
		duration = 0
	}

	return w.recorder.Start(duration)
}

func (w *FlightRecorder) stopRecording(ctx context.Context) error {
	w.logger.Debugf(ctx, "stopping flight recording")

	return w.recorder.Stop()
}

func (w *FlightRecorder) captureRecording(ctx context.Context, kind flightrecorder.Kind) error {
	if !w.currentKind.IsAllowed(kind) {
		w.logger.Tracef(ctx, "skipping capture of kind %q as current kind is %q", kind, w.currentKind)
		return nil
	}

	path := w.path
	if path == "" {
		path = "/tmp"
	}

	path, err := w.recorder.Capture(path)
	if err != nil {
		return err
	} else if path == "" {
		return nil
	}

	w.logger.Infof(ctx, "captured flight recording for %q into %q", kind, path)

	return nil
}
