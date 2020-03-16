// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitinit

import (
	"os"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"gopkg.in/juju/names.v3"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	apiuniter "github.com/juju/juju/api/uniter"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/caas/kubernetes/provider/exec"
	"github.com/juju/juju/worker/caasoperator"
)

// ManifoldConfig defines the names of the manifolds on which a
// Manifold will depend.
type ManifoldConfig struct {
	Logger Logger

	AgentName     string
	APICallerName string
	ClockName     string

	NewWorker func(Config) (worker.Worker, error)
	NewClient func(base.APICaller) Client

	NewExecClient func(namespace string) (exec.Executor, error)

	LoadOperatorInfo func(paths caasoperator.Paths) (*caas.OperatorInfo, error)
}

// Validate checks that all required configuration is provided.
func (config ManifoldConfig) Validate() error {
	if config.Logger == nil {
		return errors.NotValidf("missing Logger")
	}
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
	if config.NewExecClient == nil {
		return errors.NotValidf("missing NewExecClient")
	}
	if config.LoadOperatorInfo == nil {
		return errors.NotValidf("missing LoadOperatorInfo")
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

			var clock clock.Clock
			if err := context.Get(config.ClockName, &clock); err != nil {
				return nil, errors.Trace(err)
			}

			// Configure and start the caasoperator worker.
			agentConfig := agent.CurrentConfig()
			tag := agentConfig.Tag()
			applicationTag, ok := tag.(names.ApplicationTag)
			if !ok {
				return nil, errors.Errorf("expected an application tag, got %v", tag)
			}
			unitProviderID := func(unitTag names.UnitTag) (string, error) {
				unit, err := apiuniter.NewState(apiCaller, unitTag).Unit(unitTag)
				if err != nil {
					return "", errors.Trace(err)
				}
				return unit.ProviderID(), nil
			}

			cfg := Config{
				Logger:                config.Logger,
				Application:           applicationTag.Id(),
				Clock:                 clock,
				DataDir:               agentConfig.DataDir(),
				ContainerStartWatcher: client,
				UnitProviderIDFunc:    unitProviderID,
				NewExecClient: func() (exec.Executor, error) {
					return config.NewExecClient(os.Getenv(provider.OperatorNamespaceEnvName))
				},
				InitializeUnit: InitializeUnit,
			}
			cfg.Paths = caasoperator.NewPaths(cfg.DataDir, names.NewApplicationTag(cfg.Application))

			operatorInfo, err := config.LoadOperatorInfo(cfg.Paths)
			if err != nil {
				return nil, errors.Trace(err)
			}
			cfg.OperatorInfo = *operatorInfo

			w, err := config.NewWorker(cfg)
			if err != nil {
				return nil, errors.Trace(err)
			}
			return w, nil
		},
	}
}
