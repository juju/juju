// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster

import (
	"errors"
	"github.com/juju/juju/migration"
	"github.com/juju/juju/state"
)

// Backend defines the state functionality required by the
// migrationmaster facade.
type Backend interface {
	WatchForModelMigration() (state.NotifyWatcher, error)
	GetModelMigration() (state.ModelMigration, error)
}

var getBackend = func(st *state.State) Backend {
	return st
}

// exportModel is a shim that allows testing of the export
// functionality without requiring a real *state.State in tests.
var exportModel = func(backend Backend) ([]byte, error) {
	st, ok := backend.(*state.State)
	if !ok {
		return nil, errors.New("backend is not the expected type")
	}
	return migration.ExportModel(st)
}
