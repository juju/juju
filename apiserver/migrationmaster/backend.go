// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster

import (
	"github.com/juju/juju/migration"
	"github.com/juju/juju/state"
	"github.com/juju/version"
)

// Backend defines the state functionality required by the
// migrationmaster facade.
type Backend interface {
	WatchForMigration() state.NotifyWatcher
	LatestMigration() (state.ModelMigration, error)
	ModelUUID() string
	ModelName() (string, error)
	AgentVersion() (version.Number, error)
	RemoveExportingModelDocs() error

	migration.StateExporter
}
