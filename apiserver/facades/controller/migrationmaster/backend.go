// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster

import (
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
