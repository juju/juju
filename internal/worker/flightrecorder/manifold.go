// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package flightrecorder

import (
	"context"
	"runtime/trace"
	"time"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/core/logger"
)

// FlightRecorder is the interface for the flight recorder worker.
type FlightRecorder interface {
	// Start starts the flight recorder.
	Start() error

	// Stop stops the flight recorder.
	Stop() error

	// Capture captures a flight recording.
	Capture() error
}

// ManifoldConfig is the configuration for the flight recorder manifold.
type ManifoldConfig struct {
	FlightRecorder *trace.FlightRecorder
	Logger         logger.Logger
}

// Validate checks the configuration is valid.
func (cfg ManifoldConfig) Validate() error {
	if cfg.FlightRecorder == nil {
		return errors.NotValidf("nil FlightRecorder")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// MakeManifold creates a dependency manifold for the flight recorder worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{},
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			return NewWorker(config.FlightRecorder, config.Logger), nil
		},
		Output: func(in worker.Worker, out interface{}) error {
			w, ok := in.(*Worker)
			if !ok {
				return errors.Errorf("in should be *flightrecorder.Worker; is %T", in)
			}
			switch out := out.(type) {
			case *FlightRecorder:
				*out = w
			default:
				return errors.Errorf("out should be *flightrecorder.FlightRecorder; is %T", out)
			}

			return nil
		},
	}
}

// NewFlightRecorder creates a new flight recorder instance.
func NewFlightRecorder() *trace.FlightRecorder {
	// TODO (stickupkid): Make this configurable!
	return trace.NewFlightRecorder(trace.FlightRecorderConfig{
		MinAge:   time.Second,
		MaxBytes: 1 << 20, // 1 MiB
	})
}
