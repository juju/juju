// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstoreflag

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/engine"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/services"
)

// ManifoldConfig holds the dependencies and configuration for a
// Worker manifold.
type ManifoldConfig struct {
	AgentName               string
	ObjectStoreServicesName string
	Check                   Predicate

	NewWorker func(context.Context, Config) (worker.Worker, error)
}

// validate is called by start to check for bad configuration.
func (config ManifoldConfig) Validate() error {
	if config.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if config.ObjectStoreServicesName == "" {
		return errors.NotValidf("empty ObjectStoreServicesName")
	}
	if config.Check == nil {
		return errors.NotValidf("nil Check")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	return nil
}

// start is a StartFunc for a Worker manifold.
func (config ManifoldConfig) start(context context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var agent agent.Agent
	if err := getter.Get(config.AgentName, &agent); err != nil {
		return nil, errors.Trace(err)
	}

	var objectStoreServiceGetter services.ObjectStoreServicesGetter
	if err := getter.Get(config.ObjectStoreServicesName, &objectStoreServiceGetter); err != nil {
		return nil, errors.Trace(err)
	}

	agentConfig := agent.CurrentConfig()
	modelUUID := model.UUID(agentConfig.Model().Id())

	objectStoreService := objectStoreServiceGetter.ServicesForModel(modelUUID)

	worker, err := config.NewWorker(context, Config{
		ObjectStoreService: objectStoreService.ObjectStore(),
		ModelUUID:          modelUUID,
		Check:              config.Check,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return worker, nil
}

// Manifold packages a Worker for use in a dependency.Engine.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.ObjectStoreServicesName,
			config.AgentName,
		},
		Start:  config.start,
		Output: engine.FlagOutput,
		Filter: bounceErrChanged,
	}
}

// bounceErrChanged converts ErrChanged to dependency.ErrBounce.
func bounceErrChanged(err error) error {
	if errors.Cause(err) == ErrChanged {
		return dependency.ErrBounce
	}
	return err
}
