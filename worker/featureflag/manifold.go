// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featureflag

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"

	"github.com/juju/juju/cmd/jujud/agent/engine"
	"github.com/juju/juju/worker/common"
	"github.com/juju/juju/worker/state"
)

// ManifoldConfig holds the information necessary to run a
// featureflag.Worker in a dependency.Engine.
type ManifoldConfig struct {
	StateName string

	FlagName  string
	Invert    bool
	Logger    loggo.Logger
	NewWorker func(Config) (worker.Worker, error)
}

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.StateName == "" {
		return errors.NotValidf("empty StateName")
	}
	if config.FlagName == "" {
		return errors.NotValidf("empty FlagName")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	return nil
}

func (config ManifoldConfig) start(context dependency.Context) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var stTracker state.StateTracker
	if err := context.Get(config.StateName, &stTracker); err != nil {
		return nil, errors.Trace(err)
	}
	statePool, err := stTracker.Use()
	if err != nil {
		return nil, errors.Trace(err)
	}

	flag, err := config.NewWorker(Config{
		Source:   statePool.SystemState(),
		Logger:   config.Logger,
		FlagName: config.FlagName,
		Invert:   config.Invert,
	})
	if err != nil {
		stTracker.Done()
		return nil, errors.Trace(err)
	}
	return common.NewCleanupWorker(flag, func() { stTracker.Done() }), nil
}

// Manifold returns a dependency.Manifold that will run a FlagWorker and
// expose it to clients as a engine.Flag resource.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.StateName,
		},
		Start: config.start,
		Filter: func(err error) error {
			if errors.Cause(err) == ErrRefresh {
				return dependency.ErrBounce
			}
			return err
		},
		Output: func(in worker.Worker, out interface{}) error {
			if w, ok := in.(*common.CleanupWorker); ok {
				in = w.Worker
			}
			return engine.FlagOutput(in, out)
		},
	}
}
