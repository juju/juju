// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package diskformatter

import (
	"github.com/juju/juju/state"
	"github.com/juju/names"
)

type stateInterface interface {
	WatchBlockDevices(names.MachineTag) state.NotifyWatcher
	BlockDevices(names.MachineTag) ([]state.BlockDeviceInfo, error)
	MachineVolumeAttachments(names.MachineTag) ([]state.VolumeAttachment, error)
	VolumeAttachment(names.MachineTag, names.DiskTag) (state.VolumeAttachment, error)
	StorageInstance(names.StorageTag) (state.StorageInstance, error)
	Volume(names.DiskTag) (state.Volume, error)
}

var getState = func(st *state.State) stateInterface {
	return st
}
