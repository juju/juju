// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/proxy"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/agent/proxyupdater"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/logger"
)

// ManifoldConfig defines the names of the manifolds on which a Manifold will depend.
type ManifoldConfig struct {
	AgentName           string
	APICallerName       string
	Logger              logger.Logger
	WorkerFunc          func(Config) (worker.Worker, error)
	SupportLegacyValues bool
	ExternalUpdate      func(proxy.Settings) error
	InProcessUpdate     func(proxy.Settings) error
	RunFunc             func(string, string, ...string) (string, error)
}

// Validate ensures that all the required fields have values.
func (c ManifoldConfig) Validate() error {
	if c.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if c.APICallerName == "" {
		return errors.NotValidf("empty APICallerName")
	}
	if c.WorkerFunc == nil {
		return errors.NotValidf("nil WorkerFunc")
	}
	if c.ExternalUpdate == nil {
		return errors.NotValidf("nil ExternalUpdate")
	}
	if c.InProcessUpdate == nil {
		return errors.NotValidf("nil InProcessUpdate")
	}
	if c.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// Manifold returns a dependency manifold that runs a proxy updater worker,
// using the api connection resource named in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.APICallerName,
		},
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}
			var agent agent.Agent
			if err := getter.Get(config.AgentName, &agent); err != nil {
				return nil, err
			}
			var apiCaller base.APICaller
			if err := getter.Get(config.APICallerName, &apiCaller); err != nil {
				return nil, err
			}

			agentConfig := agent.CurrentConfig()
			proxyAPI, err := proxyupdater.NewAPI(apiCaller, agentConfig.Tag())
			if err != nil {
				return nil, err
			}
			w, err := config.WorkerFunc(Config{
				SystemdFiles:        []string{"/etc/juju-proxy-systemd.conf"},
				EnvFiles:            []string{"/etc/juju-proxy.conf"},
				API:                 proxyAPI,
				SupportLegacyValues: config.SupportLegacyValues,
				ExternalUpdate:      config.ExternalUpdate,
				InProcessUpdate:     config.InProcessUpdate,
				Logger:              config.Logger,
				RunFunc:             config.RunFunc,
			})
			if err != nil {
				return nil, errors.Trace(err)
			}
			return w, nil
		},
	}
}
