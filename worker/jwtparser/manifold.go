// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jwtparser

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	workerstate "github.com/juju/juju/worker/state"
)

// ManifoldConfig defines a Manifold's dependencies.
type ManifoldConfig struct {
	StateName string
}

// Manifold returns a manifold whose worker wraps a JWT parser.
func Manifold(config ManifoldConfig) dependency.Manifold {
	inputs := []string{config.StateName}
	return dependency.Manifold{
		Inputs: inputs,
		Output: outputFunc,
		Start:  config.start,
	}
}

// start is a method on ManifoldConfig because it's more readable than a closure.
func (config ManifoldConfig) start(context dependency.Context) (worker.Worker, error) {
	var stTracker workerstate.StateTracker
	if err := context.Get(config.StateName, &stTracker); err != nil {
		return nil, errors.Trace(err)
	}

	statePool, err := stTracker.Use()
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer func() {
		_ = stTracker.Done()
	}()

	// The statePool is only needed for worker creation.
	w, err := newWorker(statePool)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// outputFunc extracts a JWTParser from a jwtParserWorker.
func outputFunc(in worker.Worker, out interface{}) error {
	inWorker, _ := in.(*jwtParserWorker)
	if inWorker == nil {
		return errors.Errorf("in should be a %T; got %T", inWorker, in)
	}

	switch outPointer := out.(type) {
	case *Getter:
		*outPointer = inWorker
	default:
		return errors.Errorf("out should be *jwt.JWTParser; got %T", out)
	}
	return nil
}
