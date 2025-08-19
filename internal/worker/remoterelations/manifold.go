// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/worker/apicaller"
)

// ManifoldConfig defines the names of the manifolds on which a
// Worker manifold will depend.
type ManifoldConfig struct {
	AgentName     string
	APICallerName string

	NewControllerConnection  apicaller.NewExternalControllerConnectionFunc
	NewRemoteRelationsFacade func(base.APICaller) RemoteRelationsFacade
	NewWorker                func(Config) (worker.Worker, error)
	Logger                   logger.Logger
	Clock                    clock.Clock
}

// Validate is called by start to check for bad configuration.
func (config ManifoldConfig) Validate() error {
	if config.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if config.APICallerName == "" {
		return errors.NotValidf("empty APICallerName")
	}
	if config.NewControllerConnection == nil {
		return errors.NotValidf("nil NewControllerConnection")
	}
	if config.NewRemoteRelationsFacade == nil {
		return errors.NotValidf("nil NewRemoteRelationsFacade")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
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
	var apiConn api.Connection
	if err := getter.Get(config.APICallerName, &apiConn); err != nil {
		return nil, errors.Trace(err)
	}

	w, err := config.NewWorker(Config{
		ModelUUID:                agent.CurrentConfig().Model().Id(),
		RelationsFacade:          config.NewRemoteRelationsFacade(apiConn),
		NewRemoteModelFacadeFunc: remoteRelationsFacadeForModelFunc(config.NewControllerConnection),
		Clock:                    config.Clock,
		Logger:                   config.Logger,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// Manifold packages a Worker for use in a dependency.Engine.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.APICallerName,
		},
		Start: config.start,
	}
}
