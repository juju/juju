// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"github.com/juju/names"

	"github.com/juju/juju/state"
)

type storageStateInterface interface {
	StorageInstance(names.StorageTag) (state.StorageInstance, error)
	StorageAttachments(names.UnitTag) ([]state.StorageAttachment, error)
	Unit(name string) (*state.Unit, error)
}

type storageStateShim struct {
	*state.State
}

var getStorageState = func(st *state.State) storageStateInterface {
	return storageStateShim{st}
}
