// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradedatabase

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils"
	"github.com/juju/version"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/state"
	"github.com/juju/juju/upgrades"
	"github.com/juju/juju/worker/gate"
)

// ManifoldConfig defines the configuration on which this manifold depends.
type ManifoldConfig struct {
	AgentName         string
	UpgradeDBGateName string
	Logger            Logger
	OpenState         func() (*state.StatePool, error)
}

// Validate returns an error if the manifold config is not valid.
func (cfg ManifoldConfig) Validate() error {
	if cfg.UpgradeDBGateName == "" {
		return errors.NotValidf("emtpy UpgradeDBGateName")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if cfg.OpenState == nil {
		return errors.NotValidf("nil OpenState function")
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
			var upgradeStepsLock gate.Lock
			if err := context.Get(cfg.UpgradeDBGateName, &upgradeStepsLock); err != nil {
				return nil, errors.Trace(err)
			}

			// Determine this machine's agent and tag.
			var machineAgent agent.Agent
			if err := context.Get(cfg.AgentName, &machineAgent); err != nil {
				return nil, errors.Trace(err)
			}
			tag := machineAgent.CurrentConfig().Tag()

			// Wrap the state pool factory to return our implementation.
			openState := func() (Pool, error) {
				p, err := cfg.OpenState()
				if err != nil {
					return nil, errors.Trace(err)
				}
				return &pool{p}, nil
			}

			// Wrap the upgrade steps execution so that we can generate a context lazily.
			performUpgrade := func(v version.Number, t []upgrades.Target, c func() upgrades.Context) error {
				return errors.Trace(upgrades.PerformStateUpgrade(v, t, c()))
			}

			workerCfg := Config{
				UpgradeComplete: upgradeStepsLock,
				Tag:             tag,
				Agent:           machineAgent,
				Logger:          cfg.Logger,
				OpenState:       openState,
				PerformUpgrade:  performUpgrade,
				RetryStrategy:   utils.AttemptStrategy{Delay: 2 * time.Minute, Min: 5},
			}
			w, err := NewWorker(workerCfg)
			return w, errors.Annotate(err, "starting database upgrade worker")
		},
	}
}
