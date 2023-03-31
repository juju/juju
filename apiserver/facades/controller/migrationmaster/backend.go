// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster

import (
	"github.com/juju/names/v4"
	"github.com/juju/version/v2"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/migration"
	"github.com/juju/juju/state"
)

// Backend defines the state functionality required by the
// migrationmaster facade.
type Backend interface {
	migration.StateExporter

	WatchForMigration() state.NotifyWatcher
	LatestMigration() (state.ModelMigration, error)
	ModelUUID() string
	ModelName() (string, error)
	ModelOwner() (names.UserTag, error)
	AgentVersion() (version.Number, error)
	RemoveExportingModelDocs() error
	ControllerConfig() (controller.Config, error)
	APIHostPortsForClients() ([]network.SpaceHostPorts, error)
	AllLocalRelatedModels() ([]string, error)
}
