// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package flightrecorder

import (
	"context"
	"errors"

	"github.com/juju/juju/core/logger"
	"gopkg.in/tomb.v2"
)

// FlightRecorder is the flight recorder worker.
type FlightRecorder struct {
	tomb tomb.Tomb

	path     string
	recorder *Recorder
	logger   logger.Logger

	ch chan request
}

// New creates a new flight recorder worker.
func New(path string, logger logger.Logger) *FlightRecorder {
	w := &FlightRecorder{
		recorder: NewRecorder(),
		logger:   logger,

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
func (w *FlightRecorder) Start() error {
	request := request{
		Type:   requestTypeStart,
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
func (w *FlightRecorder) Capture() error {
	result := make(chan error, 1)
	req := request{
		Type:   requestTypeCapture,
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
				err = w.startRecording(ctx)
			case requestTypeStop:
				err = w.stopRecording(ctx)
			case requestTypeCapture:
				err = w.captureRecording(ctx)
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

func (w *FlightRecorder) startRecording(ctx context.Context) error {
	w.logger.Debugf(ctx, "starting flight recording")

	return w.recorder.Start()
}

func (w *FlightRecorder) stopRecording(ctx context.Context) error {
	w.logger.Debugf(ctx, "stopping flight recording")

	w.recorder.Stop()
	return nil
}

func (w *FlightRecorder) captureRecording(ctx context.Context) error {
	path := w.path
	if path == "" {
		path = "/tmp"
	}
	w.logger.Debugf(ctx, "start capturing flight recording into %q", w.path)

	path, err := w.recorder.Capture(w.path)
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
	Result chan error
}
