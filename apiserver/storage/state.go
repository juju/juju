// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import "github.com/juju/juju/state"

type storageAccess interface {
	StorageInstance(id string) (state.StorageInstance, error)
}

type stateShim struct {
	state *state.State
}

// StorageInstance calls state to get information about storage instance
func (s stateShim) StorageInstance(id string) (state.StorageInstance, error) {
	return s.state.StorageInstance(id)
}
