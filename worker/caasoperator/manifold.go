// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator

import (
	"github.com/juju/errors"
	"github.com/juju/utils/clock"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/worker/caasoperator/runner"
	"github.com/juju/juju/worker/dependency"
)

// ManifoldConfig defines the names of the manifolds on which a
// Manifold will depend.
type ManifoldConfig struct {
	AgentName     string
	APICallerName string
	ClockName     string

	NewWorker          func(Config) (worker.Worker, error)
	NewClient          func(base.APICaller) Client
	NewCharmDownloader func(base.APICaller) Downloader
}

func (config ManifoldConfig) Validate() error {
	if config.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if config.APICallerName == "" {
		return errors.NotValidf("empty APICallerName")
	}
	if config.ClockName == "" {
		return errors.NotValidf("empty ClockName")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("missing NewWorker")
	}
	if config.NewClient == nil {
		return errors.NotValidf("missing NewClient")
	}
	if config.NewCharmDownloader == nil {
		return errors.NotValidf("missing NewCharmDownloader")
	}
	return nil
}

// Manifold returns a dependency manifold that runs a caasoperator worker,
// using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.APICallerName,
			config.ClockName,
		},
		Start: func(context dependency.Context) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			var agent agent.Agent
			if err := context.Get(config.AgentName, &agent); err != nil {
				return nil, errors.Trace(err)
			}

			var apiCaller base.APICaller
			if err := context.Get(config.APICallerName, &apiCaller); err != nil {
				return nil, errors.Trace(err)
			}
			client := config.NewClient(apiCaller)
			downloader := config.NewCharmDownloader(apiCaller)

			var clock clock.Clock
			if err := context.Get(config.ClockName, &clock); err != nil {
				return nil, errors.Trace(err)
			}

			modelName, err := client.ModelName()
			if err != nil {
				return nil, errors.Trace(err)
			}

			// Configure and start the caasoperator worker.
			agentConfig := agent.CurrentConfig()
			tag := agentConfig.Tag()
			applicationTag, ok := tag.(names.ApplicationTag)
			if !ok {
				return nil, errors.Errorf("expected an application tag, got %v", tag)
			}
			w, err := config.NewWorker(Config{
				ModelUUID:               agentConfig.Model().Id(),
				ModelName:               modelName,
				NewRunnerFactoryFunc:    runner.NewFactory,
				Application:             applicationTag.Id(),
				ApplicationConfigGetter: client,
				CharmGetter:             client,
				Clock:                   clock,
				ContainerSpecSetter:     client,
				DataDir:                 agentConfig.DataDir(),
				Downloader:              downloader,
				StatusSetter:            client,
				APIAddressGetter:        client,
				ProxySettingsGetter:     client,
			})
			if err != nil {
				return nil, errors.Trace(err)
			}
			return w, nil
		},
	}
}
