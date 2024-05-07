// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dualwritehack

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/internal/servicefactory"
	"github.com/juju/juju/state"
)

// ManifoldConfig describes the resources and configuration on which the
// dualwritehack worker depends.
type ManifoldConfig struct {
	ServiceFactoryName string
	StatePool          *state.StatePool
	Logger             Logger
}

// Validate is called by start to check for bad configuration.
func (config ManifoldConfig) Validate() error {
	if config.ServiceFactoryName == "" {
		return errors.NotValidf("empty ServiceFactoryName")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// Manifold returns a Manifold that encapsulates the statushistorypruner worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.ServiceFactoryName,
		},
		Start: config.start,
	}
}

// start is a StartFunc for a Worker manifold.
func (config ManifoldConfig) start(context context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var serviceFactory servicefactory.ServiceFactory
	if err := getter.Get(config.ServiceFactoryName, &serviceFactory); err != nil {
		return nil, errors.Trace(err)
	}

	prunerConfig := Config{
		StatePool:      config.StatePool,
		ServiceFactory: serviceFactory,
		Logger:         config.Logger,
	}
	w, err := NewWorker(prunerConfig)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}
