// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/errors"
	"github.com/juju/juju/state"

	"github.com/juju/juju/apiserver/params"
)

type storageAccess interface {
	Show(entities params.Entities) (params.StorageInstancesResult, error)
}

type stateShim struct {
	state *state.State
}

// Show calls state to get information about storage instances
func (s stateShim) Show(entities params.Entities) (params.StorageInstancesResult, error) {
	nothing := params.StorageInstancesResult{}
	// TODO(anastasiamac) plug into a real implementation. This is just a placeholder.
	return nothing, errors.NotImplementedf("This needs to plug into a real deal!")
}
