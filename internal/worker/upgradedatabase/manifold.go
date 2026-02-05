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
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/semversion"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/worker/gate"
	"github.com/juju/juju/internal/worker/upgradedatabase/upgradesteps"
)

// UpgradeStep is a function that performs a single step in the database upgrade
// process. It is provided with the controller and model databases to perform
// any necessary migrations or transformations.
type UpgradeStep func(ctx context.Context, controllerDB, modelDB coredatabase.TxnRunner, modelUUID model.UUID) error

// ManifoldConfig defines the configuration on which this manifold depends.
type ManifoldConfig struct {
	AgentName           string
	UpgradeDBGateName   string
	DBAccessorName      string
	UpgradeServicesName string

	Logger    logger.Logger
	Clock     clock.Clock
	NewWorker func(Config) (worker.Worker, error)

	UpgradeSteps map[VersionWindow][]UpgradeStep
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
	if cfg.UpgradeServicesName == "" {
		return errors.NotValidf("empty UpgradeServicesName")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if cfg.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if cfg.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if cfg.UpgradeSteps == nil {
		return errors.NotValidf("nil UpgradeSteps")
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

			// Work out where we're upgrading from and, where we want to upgrade
			// to.
			fromVersion := currentConfig.UpgradedToVersion()
			toVersion := jujuversion.Current

			return cfg.NewWorker(Config{
				DBUpgradeCompleteLock: dbUpgradeCompleteLock,
				Agent:                 controllerAgent,
				ControllerNodeService: upgradeServicesGetter.
					ServicesForController().ControllerNode(),
				UpgradeService: upgradeServicesGetter.
					ServicesForController().Upgrade(),
				UpgradeSteps: filterSteps(cfg.UpgradeSteps, fromVersion),
				DBGetter:     dbGetter,
				Tag:          currentConfig.Tag(),
				FromVersion:  fromVersion,
				ToVersion:    toVersion,
				Logger:       cfg.Logger,
				Clock:        cfg.Clock,
			})
		},
	}
}

// VersionWindow defines a window of versions from From (inclusive) to To
// (exclusive).
type VersionWindow struct {
	From semversion.Number
	To   semversion.Number
}

// Includes returns true if the version window includes the specified version.
func (w VersionWindow) Includes(v semversion.Number) bool {
	patch := v.ToPatch()
	return patch.Compare(w.From) >= 0 && patch.Compare(w.To) < 0
}

var (
	window_4_0_0_to_4_0_1 = VersionWindow{
		From: semversion.MustParse("4.0.0"),
		To:   semversion.MustParse("4.0.1"),
	}
	window_4_0_1_to_4_0_2 = VersionWindow{
		From: semversion.MustParse("4.0.1"),
		To:   semversion.MustParse("4.0.2"),
	}
)

// UpgradeSteps returns the mapping of upgrade steps for the database upgrade
// process.
//
// The upgrade steps defines from which version it's upgrading *from*. It's
// possible you can step over multiple versions in one upgrade (4.0.0 to
// 4.0.10). As the upgrade process only needs to know what steps to fix, not
// what versions are required to step through.
var UpgradeSteps = map[VersionWindow][]UpgradeStep{
	window_4_0_0_to_4_0_1: {
		upgradesteps.Step0001_PatchModelConfigCloudType,
	},
	window_4_0_1_to_4_0_2: {
		upgradesteps.Step0002_RemoveLXDSubnetProviderID,
	},
}

// filterSteps filters the upgrade steps to only those that are applicable
// for the upgrade from the specified version.
func filterSteps(allSteps map[VersionWindow][]UpgradeStep, fromVersion semversion.Number) []UpgradeStep {
	var steps []UpgradeStep
	for version, upgradeSteps := range allSteps {
		if version.Includes(fromVersion) {
			steps = append(steps, upgradeSteps...)
		}
	}
	return steps
}
