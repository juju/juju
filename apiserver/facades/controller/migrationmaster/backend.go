// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster

import (
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/migration"
	"github.com/juju/juju/state"
)

// Backend defines the state functionality required by the
// migrationmaster facade.
type Backend interface {
	migration.LegacyStateExporter

	WatchForMigration() state.NotifyWatcher
	LatestMigration() (state.ModelMigration, error)
	RemoveExportingModelDocs() error
}

// ControllerState defines the state functionality for controller model.
type ControllerState interface {
	APIHostPortsForClients(controller.Config) ([]network.SpaceHostPorts, error)
}
