// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package flightrecorder

import (
	"context"
	"errors"

	"github.com/juju/juju/core/flightrecorder"
	"github.com/juju/juju/core/logger"
	"gopkg.in/tomb.v2"
)

// FileRecorder defines the interface for a flight recorder that can
// start, stop and capture recordings to a file.
type FileRecorder interface {
	// Start starts the flight recorder.
	Start() error

	// Stop stops the flight recorder.
	Stop() error

	// Capture captures a flight recording to the given path.
	Capture(path string) (string, error)
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
	logger   logger.Logger

	currentKind flightrecorder.Kind
	ch          chan request
}

// New creates a new flight recorder worker.
func New(recorder FileRecorder, path string, logger logger.Logger) *FlightRecorder {
	w := &FlightRecorder{
		recorder:    recorder,
		path:        path,
		logger:      logger,
		currentKind: flightrecorder.KindAny,

		ch: make(chan request),
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

// Start starts the flight recorder.
func (w *FlightRecorder) Start(kind flightrecorder.Kind) error {
	request := request{
		Type:   requestTypeStart,
		Kind:   kind,
		Result: make(chan error, 1),
	}

	select {
	case <-w.tomb.Dying():
		return errors.New("worker is stopping")
	case w.ch <- request:
	}

	select {
	case <-w.tomb.Dying():
		return errors.New("worker is stopping")
	case err := <-request.Result:
		return err
	}
}

// Stop stops the flight recorder.
func (w *FlightRecorder) Stop() error {
	result := make(chan error, 1)
	req := request{
		Type:   requestTypeStop,
		Result: result,
	}

	select {
	case <-w.tomb.Dying():
		return errors.New("worker is stopping")
	case w.ch <- req:
	}

	select {
	case <-w.tomb.Dying():
		return errors.New("worker is stopping")
	case err := <-result:
		return err
	}
}

// Capture captures a flight recording.
func (w *FlightRecorder) Capture(kind flightrecorder.Kind) error {
	result := make(chan error, 1)
	req := request{
		Type:   requestTypeCapture,
		Kind:   kind,
		Result: result,
	}

	select {
	case <-w.tomb.Dying():
		return errors.New("worker is stopping")
	case w.ch <- req:
	}

	select {
	case <-w.tomb.Dying():
		return errors.New("worker is stopping")
	case err := <-result:
		return err
	}
}

func (w *FlightRecorder) loop() error {
	ctx := w.tomb.Context(context.Background())

	defer w.recorder.Stop()

	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying

		case req := <-w.ch:
			var err error
			switch req.Type {
			case requestTypeStart:
				err = w.startRecording(ctx, req.Kind)
			case requestTypeStop:
				err = w.stopRecording(ctx)
			case requestTypeCapture:
				err = w.captureRecording(ctx, req.Kind)
			default:
				err = errors.New("unknown request type")
			}

			select {
			case <-w.tomb.Dying():
				return tomb.ErrDying
			case req.Result <- err:
			}
		}
	}
}

func (w *FlightRecorder) startRecording(ctx context.Context, kind flightrecorder.Kind) error {
	w.logger.Debugf(ctx, "starting flight recording for kind %q", kind)

	w.currentKind = kind

	return w.recorder.Start()
}

func (w *FlightRecorder) stopRecording(ctx context.Context) error {
	w.logger.Debugf(ctx, "stopping flight recording")

	w.recorder.Stop()
	return nil
}

func (w *FlightRecorder) captureRecording(ctx context.Context, kind flightrecorder.Kind) error {
	if !w.currentKind.IsAllowed(kind) {
		w.logger.Debugf(ctx, "skipping capture of kind %q as current kind is %q", kind, w.currentKind)
		return nil
	}

	path := w.path
	if path == "" {
		path = "/tmp"
	}
	w.logger.Debugf(ctx, "start capturing flight recording into %q", path)

	path, err := w.recorder.Capture(path)
	if err != nil {
		return err
	}

	w.logger.Infof(ctx, "captured flight recording into %q", path)

	return nil
}

type requestType int

const (
	requestTypeStart requestType = iota
	requestTypeStop
	requestTypeCapture
)

type request struct {
	Type   requestType
	Kind   flightrecorder.Kind
	Result chan error
}
