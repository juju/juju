// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations

import (
	"github.com/juju/errors"
	worker "gopkg.in/juju/worker.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/worker/dependency"
)

// ManifoldConfig defines the names of the manifolds on which a
// Worker manifold will depend.
type ManifoldConfig struct {
	AgentName     string
	APICallerName string

	NewAPIConnForModel       api.NewConnectionForModelFunc
	NewRemoteRelationsFacade func(base.APICaller) (RemoteRelationsFacade, error)
	NewWorker                func(Config) (worker.Worker, error)
}

// Validate is called by start to check for bad configuration.
func (config ManifoldConfig) Validate() error {
	if config.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if config.APICallerName == "" {
		return errors.NotValidf("empty APICallerName")
	}
	if config.NewAPIConnForModel == nil {
		return errors.NotValidf("nil NewAPIConnForModel")
	}
	if config.NewRemoteRelationsFacade == nil {
		return errors.NotValidf("nil NewRemoteRelationsFacade")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	return nil
}

// start is a StartFunc for a Worker manifold.
func (config ManifoldConfig) start(context dependency.Context) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var agent agent.Agent
	if err := context.Get(config.AgentName, &agent); err != nil {
		return nil, errors.Trace(err)
	}
	var apiConn api.Connection
	if err := context.Get(config.APICallerName, &apiConn); err != nil {
		return nil, errors.Trace(err)
	}
	facade, err := config.NewRemoteRelationsFacade(apiConn)
	if err != nil {
		return nil, errors.Trace(err)
	}

	agentConf := agent.CurrentConfig()
	apiInfo, ok := agentConf.APIInfo()
	if !ok {
		return nil, errors.New("no API connection details")
	}
	// We use an anonymous connection and macaroon sent with the
	// publish requests for authentication.
	apiInfo.Tag = nil
	apiInfo.Password = ""
	apiInfo.Macaroons = nil
	apiConnForModelFunc, err := config.NewAPIConnForModel(apiInfo)
	if err != nil {
		return nil, errors.Trace(err)
	}

	w, err := config.NewWorker(Config{
		ModelUUID:                agent.CurrentConfig().Model().Id(),
		RelationsFacade:          facade,
		NewPublisherForModelFunc: relationChangePublisherForModelFunc(apiConnForModelFunc),
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
