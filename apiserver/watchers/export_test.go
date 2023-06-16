// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watchers

import "github.com/juju/juju/state"

// PatchGetMigrationBackend overrides the getMigrationBackend function
// to support testing.
func PatchGetMigrationBackend(p Patcher, ctrlSt controllerBackend, st migrationBackend) {
	p.PatchValue(&getMigrationBackend, func(*state.State) migrationBackend {
		return st
	})
	p.PatchValue(&getControllerBackend, func(pool *state.StatePool) (controllerBackend, error) {
		return ctrlSt, nil
	})
}

// PatchGetControllerCACert overrides the getControllerCACert function
// to support testing.
func PatchGetControllerCACert(p Patcher, caCert string) {
	p.PatchValue(&getControllerCACert, func(backend controllerBackend) (string, error) {
		return caCert, nil
	})
}

// Patcher defines an interface that matches the PatchValue method on
// CleanupSuite
type Patcher interface {
	PatchValue(ptr, value interface{})
}
