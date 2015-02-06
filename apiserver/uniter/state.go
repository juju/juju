// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"github.com/juju/juju/state"
)

type storageStateInterface interface {
	StorageInstance(id string) (state.StorageInstance, error)
	Unit(name string) (*state.Unit, error)
}

type storageStateShim struct {
	*state.State
}

var getStorageState = func(st *state.State) storageStateInterface {
	return storageStateShim{st}
}

func (s storageStateShim) StorageInstance(id string) (state.StorageInstance, error) {
	return s.State.StorageInstance(id)
}

func (s storageStateShim) Unit(name string) (*state.Unit, error) {
	return s.State.Unit(name)
}
