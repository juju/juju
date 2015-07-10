// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/state"
)

type provisionerState interface {
	state.EntityFinder
	state.EnvironAccessor

	MachineInstanceId(names.MachineTag) (instance.Id, error)
	BlockDevices(names.MachineTag) ([]state.BlockDeviceInfo, error)

	WatchBlockDevices(names.MachineTag) state.NotifyWatcher
	WatchMachine(names.MachineTag) (state.NotifyWatcher, error)
	WatchEnvironFilesystems() state.StringsWatcher
	WatchEnvironFilesystemAttachments() state.StringsWatcher
	WatchMachineFilesystems(names.MachineTag) state.StringsWatcher
	WatchMachineFilesystemAttachments(names.MachineTag) state.StringsWatcher
	WatchEnvironVolumes() state.StringsWatcher
	WatchEnvironVolumeAttachments() state.StringsWatcher
	WatchMachineVolumes(names.MachineTag) state.StringsWatcher
	WatchMachineVolumeAttachments(names.MachineTag) state.StringsWatcher
	WatchVolumeAttachment(names.MachineTag, names.VolumeTag) state.NotifyWatcher

	StorageInstance(names.StorageTag) (state.StorageInstance, error)

	Filesystem(names.FilesystemTag) (state.Filesystem, error)
	FilesystemAttachment(names.MachineTag, names.FilesystemTag) (state.FilesystemAttachment, error)

	Volume(names.VolumeTag) (state.Volume, error)
	VolumeAttachment(names.MachineTag, names.VolumeTag) (state.VolumeAttachment, error)
	VolumeAttachments(names.VolumeTag) ([]state.VolumeAttachment, error)

	RemoveFilesystem(names.FilesystemTag) error
	RemoveFilesystemAttachment(names.MachineTag, names.FilesystemTag) error
	RemoveVolume(names.VolumeTag) error
	RemoveVolumeAttachment(names.MachineTag, names.VolumeTag) error

	SetFilesystemInfo(names.FilesystemTag, state.FilesystemInfo) error
	SetFilesystemAttachmentInfo(names.MachineTag, names.FilesystemTag, state.FilesystemAttachmentInfo) error
	SetVolumeInfo(names.VolumeTag, state.VolumeInfo) error
	SetVolumeAttachmentInfo(names.MachineTag, names.VolumeTag, state.VolumeAttachmentInfo) error
}

type stateShim struct {
	*state.State
}

func (s stateShim) MachineInstanceId(tag names.MachineTag) (instance.Id, error) {
	m, err := s.Machine(tag.Id())
	if err != nil {
		return "", errors.Trace(err)
	}
	return m.InstanceId()
}

func (s stateShim) WatchMachine(tag names.MachineTag) (state.NotifyWatcher, error) {
	m, err := s.Machine(tag.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return m.Watch(), nil
}
