// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package diskformatter

import (
	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/state"
	"github.com/juju/names"
)

type stateInterface interface {
	WatchUnitMachineBlockDevices(names.UnitTag) (watcher.StringsWatcher, error)
	BlockDevice(name string) (state.BlockDevice, error)
	StorageInstance(id string) (state.StorageInstance, error)
}

var getState = func(st *state.State) stateInterface {
	return stateShim{st}
}

type stateShim struct {
	*state.State
}

func (s stateShim) WatchUnitMachineBlockDevices(
	tag names.UnitTag,
) (watcher.StringsWatcher, error) {
	u, err := s.State.Unit(tag.Id())
	if err != nil {
		return nil, err
	}
	mid, err := u.AssignedMachineId()
	if err != nil {
		return nil, err
	}
	m, err := s.State.Machine(mid)
	if err != nil {
		return nil, err
	}
	return m.WatchBlockDevices(), nil
}
