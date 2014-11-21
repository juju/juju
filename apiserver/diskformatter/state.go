// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package diskformatter

import (
	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
)

type stateInterface interface {
	WatchAttachedBlockDevices(unit string) (watcher.NotifyWatcher, error)
	AttachedBlockDevices(unit string) ([]storage.BlockDevice, error)
}

type stateShim struct {
	*state.State
}

func (s stateShim) WatchAttachedBlockDevices(unit string) (watcher.NotifyWatcher, error) {
	u, err := s.State.Unit(unit)
	if err != nil {
		return nil, err
	}
	return u.WatchAttachedBlockDevices()
}

func (s stateShim) AttachedBlockDevices(unit string) ([]storage.BlockDevice, error) {
	u, err := s.State.Unit(unit)
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
	// TODO(axw) attached only
	return m.BlockDevices()
}
