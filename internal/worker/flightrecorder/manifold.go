// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package flightrecorder

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/agent/engine"
	"github.com/juju/juju/core/flightrecorder"
)

// Manifold returns a dependency manifold for the flight recorder worker.
func Manifold(flightRecorder flightrecorder.FlightRecorderWorker) dependency.Manifold {
	return dependency.Manifold{
		Start: func(_ context.Context, _ dependency.Getter) (worker.Worker, error) {
			return engine.NewOwnedWorker(flightRecorder)
		},
		Output: func(in worker.Worker, out interface{}) error {
			recorder, ok := in.(flightrecorder.FlightRecorderWorker)
			if !ok {
				return errors.NotValidf("expected flightrecorder.FlightRecorderWorker, got %T", in)
			}
			switch out := out.(type) {
			case *flightrecorder.FlightRecorderWorker:
				*out = recorder
			case *flightrecorder.FlightRecorder:
				*out = recorder
			default:
				return errors.NotValidf("expected *flightrecorder.FlightRecorderWorker, got %T", out)
			}
			return nil
		},
	}
}
