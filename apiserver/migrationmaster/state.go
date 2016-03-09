// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster

import "github.com/juju/juju/state"

var getBackend = func(st *state.State) Backend {
	return st
}

// Backend defines the state functionality required by the
// migrationmaster facade.
type Backend interface {
	WatchForModelMigration() (state.NotifyWatcher, error)
	GetModelMigration() (state.ModelMigration, error)
}
