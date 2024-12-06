// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasenvironupgrader

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"

	jujuworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/gate"
)

// Logger is here to stop the desire of creating a package level Logger.
// Don't do this, instead use the one passed as manifold config.
type logger interface{}

var _ logger = struct{}{}

// Config holds the configuration and dependencies for a worker.
type Config struct {
	// GateUnlocker holds a gate.Unlocker that the worker must call
	// after the model has been successfully upgraded.
	GateUnlocker gate.Unlocker
}

// Validate returns an error if the config cannot be expected
// to drive a functional worker.
func (config Config) Validate() error {
	if config.GateUnlocker == nil {
		return errors.NotValidf("nil GateUnlocker")
	}
	return nil
}

// NewWorker returns a worker that unlocks the model upgrade gate.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	// There are no upgrade steps for a CAAS model.
	// We just set the status to available and unlock the gate.
	return jujuworker.NewSimpleWorker(func(ctx context.Context) error {
		config.GateUnlocker.Unlock()
		return nil
	}), nil
}
