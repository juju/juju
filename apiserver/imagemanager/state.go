// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemanager

import (
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/imagestorage"
	names "gopkg.in/juju/names.v2"
)

type stateInterface interface {
	ImageStorage() imagestorage.Storage
	ControllerTag() names.ControllerTag
}

type stateShim struct {
	*state.State
}

func (s stateShim) ImageStorage() imagestorage.Storage {
	return s.State.ImageStorage()
}
