// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package services

import (
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/logger"
	upgradeservice "github.com/juju/juju/domain/upgrade/service"
	upgradestate "github.com/juju/juju/domain/upgrade/state"
)

// UpgradeServices provides access to the services required for performing a
// controller upgrade.
type UpgradeServices struct {
	serviceFactoryBase
}

// NewUpgradeServices returns a new registry for accessing services related to
// upgrading controllers.
func NewUpgradeServices(
	controllerDB changestream.WatchableDBFactory,
	logger logger.Logger,
) *UpgradeServices {
	return &UpgradeServices{
		serviceFactoryBase: serviceFactoryBase{
			controllerDB: controllerDB,
			logger:       logger,
		},
	}
}

// Upgrade returns the upgrade service.
func (s *UpgradeServices) Upgrade() *upgradeservice.WatchableService {
	return upgradeservice.NewWatchableService(
		upgradestate.NewState(changestream.NewTxnRunnerFactory(s.controllerDB)),
		s.controllerWatcherFactory("upgrade"),
	)
}
