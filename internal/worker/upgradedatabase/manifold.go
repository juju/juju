// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradedatabase

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/agent"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/worker/gate"
)

// ManifoldConfig defines the configuration on which this manifold depends.
type ManifoldConfig struct {
	AgentName           string
	UpgradeDBGateName   string
	DBAccessorName      string
	Logger              logger.Logger
	Clock               clock.Clock
	NewWorker           func(Config) (worker.Worker, error)
	UpgradeServicesName string
}

// Validate returns an error if the manifold config is not valid.
func (cfg ManifoldConfig) Validate() error {
	if cfg.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if cfg.UpgradeDBGateName == "" {
		return errors.NotValidf("empty UpgradeDBGateName")
	}

	if cfg.DBAccessorName == "" {
		return errors.NotValidf("empty DBAccessorName")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if cfg.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if cfg.UpgradeServicesName == "" {
		return errors.NotValidf("empty UpgradeServicesName")
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
			cfg.UpgradeServicesName,
			cfg.DBAccessorName,
		},
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			// Get the db completed lock.
			var dbUpgradeCompleteLock gate.Lock
			if err := getter.Get(cfg.UpgradeDBGateName, &dbUpgradeCompleteLock); err != nil {
				return nil, errors.Trace(err)
			}

			// Determine this controller's agent.
			var controllerAgent agent.Agent
			if err := getter.Get(cfg.AgentName, &controllerAgent); err != nil {
				return nil, errors.Trace(err)
			}

			// Service factory is used to get the upgrade service and
			// then we can locate all the model uuids.
			var upgradeServicesGetter services.UpgradeServicesGetter
			err := getter.Get(cfg.UpgradeServicesName, &upgradeServicesGetter)
			if err != nil {
				return nil, errors.Trace(err)
			}

			// DBGetter is used to get the database to run the schema against.
			var dbGetter coredatabase.DBGetter
			if err := getter.Get(cfg.DBAccessorName, &dbGetter); err != nil {
				return nil, errors.Trace(err)
			}

			currentConfig := controllerAgent.CurrentConfig()

			// Work out where we're upgrading from and, where we want to upgrade to.
			fromVersion := currentConfig.UpgradedToVersion()
			toVersion := jujuversion.Current

			return cfg.NewWorker(Config{
				DBUpgradeCompleteLock: dbUpgradeCompleteLock,
				Agent:                 controllerAgent,
				UpgradeService:        upgradeServicesGetter.ServicesForController().Upgrade(),
				DBGetter:              dbGetter,
				Tag:                   currentConfig.Tag(),
				FromVersion:           fromVersion,
				ToVersion:             toVersion,
				Logger:                cfg.Logger,
				Clock:                 cfg.Clock,
			})
		},
	}
}
