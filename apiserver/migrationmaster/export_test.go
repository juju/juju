// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster

import (
	"github.com/juju/juju/migration"
	"github.com/juju/juju/state"
)

func PatchState(p Patcher, st Backend) {
	p.PatchValue(&getBackend, func(*state.State) Backend {
		return st
	})
}

func PatchExportModel(p Patcher, f func(migration.StateExporter) ([]byte, error)) {
	p.PatchValue(&exportModel, f)
}

type Patcher interface {
	PatchValue(ptr, value interface{})
}
