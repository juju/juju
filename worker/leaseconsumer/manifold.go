// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package leaseconsumer

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/apiserver/httpcontext"
)

// ManifoldConfig holds the information necessary to run an apiserver-based
// lease consumer worker in a dependency.Engine.
type ManifoldConfig struct {
	AgentName         string
	AuthenticatorName string
	MuxName           string

	NewWorker func(Config) (worker.Worker, error)

	// Path is the path of the lease HTTP endpoint.
	Path string
}

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if config.AuthenticatorName == "" {
		return errors.NotValidf("empty AuthenticatorName")
	}
	if config.MuxName == "" {
		return errors.NotValidf("empty MuxName")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.Path == "" {
		return errors.NotValidf("empty Path")
	}
	return nil
}

// Manifold returns a dependency.Manifold that will run an apiserver-based
// raft transport worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.AuthenticatorName,
			config.MuxName,
		},
		Start: config.start,
	}
}

func (config ManifoldConfig) start(context dependency.Context) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var agent agent.Agent
	if err := context.Get(config.AgentName, &agent); err != nil {
		return nil, errors.Trace(err)
	}

	var authenticator httpcontext.Authenticator
	if err := context.Get(config.AuthenticatorName, &authenticator); err != nil {
		return nil, errors.Trace(err)
	}

	var mux *apiserverhttp.Mux
	if err := context.Get(config.MuxName, &mux); err != nil {
		return nil, errors.Trace(err)
	}

	return config.NewWorker(Config{
		Authenticator: authenticator,
		Mux:           mux,
		Path:          config.Path,
	})
}
