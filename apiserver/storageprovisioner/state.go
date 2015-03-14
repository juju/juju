// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"github.com/juju/names"

	"github.com/juju/juju/state"
)

type provisionerState interface {
	state.EntityFinder
	WatchEnvironVolumes() state.StringsWatcher
	WatchMachineVolumes(names.MachineTag) state.StringsWatcher
	WatchMachineVolumeAttachments(names.MachineTag) state.StringsWatcher
	Volume(names.VolumeTag) (state.Volume, error)
	VolumeAttachments(names.VolumeTag) ([]state.VolumeAttachment, error)
	SetVolumeInfo(names.VolumeTag, state.VolumeInfo) error
}

type stateShim struct {
	*state.State
}
