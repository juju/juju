// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationminion

import "github.com/juju/juju/state"

// Backend defines the state functionality required by the
// MigrationMinion facade.
type Backend interface {
	WatchMigrationStatus() state.NotifyWatcher
	Migration(string) (state.ModelMigration, error)
}
