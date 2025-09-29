// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package flightrecorder

import (
	"context"
	"errors"
	"os"
	"runtime/trace"
	"time"

	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/logger"
)

// Worker is the flight recorder worker.
type Worker struct {
	tomb tomb.Tomb

	logger logger.Logger

	ch chan request
}

// NewWorker creates a new flight recorder worker.
func NewWorker(logger logger.Logger) *Worker {
	w := &Worker{
		logger: logger,
	}

	w.tomb.Go(w.loop)

	return w
}

// Kill stops the worker.
func (w *Worker) Kill() {
	w.tomb.Kill(nil)
}

// Wait waits for the worker to stop.
func (w *Worker) Wait() error {
	return w.tomb.Wait()
}

// Start starts the flight recorder.
func (w *Worker) Start() error {
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
func (w *Worker) Stop() error {
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
func (w *Worker) Capture() error {
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

func (w *Worker) loop() error {
	ctx := w.tomb.Context(context.Background())

	// TODO (stickupkid): Make this configurable!
	flightRecorder := trace.NewFlightRecorder(trace.FlightRecorderConfig{
		MinAge:   time.Second,
		MaxBytes: 1 << 20, // 1 MiB
	})

	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying

		case req := <-w.ch:
			var err error
			switch req.Type {
			case requestTypeStart:
				err = w.startRecording(ctx, flightRecorder)
			case requestTypeStop:
				err = w.stopRecording(ctx, flightRecorder)
			case requestTypeCapture:
				err = w.captureRecording(ctx, flightRecorder)
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

func (w *Worker) startRecording(ctx context.Context, recorder *trace.FlightRecorder) error {
	w.logger.Debugf(ctx, "starting flight recording")

	return recorder.Start()
}

func (w *Worker) stopRecording(ctx context.Context, recorder *trace.FlightRecorder) error {
	w.logger.Debugf(ctx, "stopping flight recording")

	recorder.Stop()
	return nil
}

func (w *Worker) captureRecording(ctx context.Context, recorder *trace.FlightRecorder) error {
	if !recorder.Enabled() {
		return nil
	}

	defer recorder.Stop()

	f, err := os.CreateTemp("", "flight_recording.trace")
	if err != nil {
		return err
	}
	defer f.Close()

	w.logger.Debugf(ctx, "capturing flight recording to %q", f.Name())

	if _, err := recorder.WriteTo(f); err != nil {
		return err
	}

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
