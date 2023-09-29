// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradedatabase

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/worker/gate"
)

// Logger defines the logging methods used by the worker.
type Logger interface {
	Errorf(string, ...any)
	Warningf(string, ...any)
	Infof(string, ...any)
	Debugf(string, ...any)
}

// ManifoldConfig defines the configuration on which this manifold depends.
type ManifoldConfig struct {
	AgentName         string
	UpgradeDBGateName string
	Logger            Logger
}

// Validate returns an error if the manifold config is not valid.
func (cfg ManifoldConfig) Validate() error {
	if cfg.UpgradeDBGateName == "" {
		return errors.NotValidf("empty UpgradeDBGateName")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// Manifold returns a dependency manifold that runs a database upgrade worker
// using the resource names defined in the supplied config.
func Manifold(cfg ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			cfg.AgentName,
			cfg.UpgradeDBGateName,
		},
		Start: func(context dependency.Context) (worker.Worker, error) {
			// Get the completed lock.
			var upgradeDatabaseLock gate.Lock
			if err := context.Get(cfg.UpgradeDBGateName, &upgradeDatabaseLock); err != nil {
				return nil, errors.Trace(err)
			}

			// Determine this controller's agent.
			var controllerAgent agent.Agent
			if err := context.Get(cfg.AgentName, &controllerAgent); err != nil {
				return nil, errors.Trace(err)
			}

			workerCfg := Config{
				UpgradeComplete: upgradeDatabaseLock,
				Agent:           controllerAgent,
				Logger:          cfg.Logger,
			}
			w, err := NewUpgradeDatabaseWorker(workerCfg)
			return w, errors.Annotate(err, "starting database upgrade worker")
		},
	}
}
