// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/state"
)

// This file contains untested shims to let us wrap state in a sensible
// interface and avoid writing tests that depend on mongodb. If you were
// to change any part of it so that it were no longer *obviously* and
// *trivially* correct, you would be Doing It Wrong.

type Backend interface {
	state.EntityFinder

	WatchMachine(names.MachineTag) (state.NotifyWatcher, error)
}

type StorageBackend interface {
	WatchModelFilesystems() state.StringsWatcher
	WatchModelFilesystemAttachments() state.StringsWatcher
	WatchMachineFilesystems(names.MachineTag) state.StringsWatcher
	WatchMachineFilesystemAttachments(names.MachineTag) state.StringsWatcher
	WatchUnitFilesystemAttachments(tag names.ApplicationTag) state.StringsWatcher
	WatchModelVolumes() state.StringsWatcher
	WatchModelVolumeAttachments() state.StringsWatcher
	WatchMachineVolumes(names.MachineTag) state.StringsWatcher
	WatchMachineVolumeAttachments(names.MachineTag) state.StringsWatcher
	WatchUnitVolumeAttachments(tag names.ApplicationTag) state.StringsWatcher
	WatchVolumeAttachment(names.Tag, names.VolumeTag) state.NotifyWatcher
	WatchMachineAttachmentsPlans(names.MachineTag) state.StringsWatcher

	StorageInstance(names.StorageTag) (state.StorageInstance, error)
	AllStorageInstances() ([]state.StorageInstance, error)
	StorageInstanceVolume(names.StorageTag) (state.Volume, error)
	StorageInstanceFilesystem(names.StorageTag) (state.Filesystem, error)
	ReleaseStorageInstance(names.StorageTag, bool, bool, time.Duration) error
	DetachStorage(names.StorageTag, names.UnitTag, bool, time.Duration) error

	Filesystem(names.FilesystemTag) (state.Filesystem, error)
	FilesystemAttachment(names.Tag, names.FilesystemTag) (state.FilesystemAttachment, error)

	Volume(names.VolumeTag) (state.Volume, error)
	VolumeAttachment(names.Tag, names.VolumeTag) (state.VolumeAttachment, error)
	VolumeAttachments(names.VolumeTag) ([]state.VolumeAttachment, error)
	VolumeAttachmentPlan(names.Tag, names.VolumeTag) (state.VolumeAttachmentPlan, error)
	VolumeAttachmentPlans(volume names.VolumeTag) ([]state.VolumeAttachmentPlan, error)

	RemoveFilesystem(names.FilesystemTag) error
	RemoveFilesystemAttachment(names.Tag, names.FilesystemTag, bool) error
	RemoveVolume(names.VolumeTag) error
	RemoveVolumeAttachment(names.Tag, names.VolumeTag, bool) error
	DetachFilesystem(names.Tag, names.FilesystemTag) error
	DestroyFilesystem(names.FilesystemTag, bool) error
	DetachVolume(names.Tag, names.VolumeTag, bool) error
	DestroyVolume(names.VolumeTag, bool) error

	SetFilesystemInfo(names.FilesystemTag, state.FilesystemInfo) error
	SetFilesystemAttachmentInfo(names.Tag, names.FilesystemTag, state.FilesystemAttachmentInfo) error
	SetVolumeInfo(names.VolumeTag, state.VolumeInfo) error
	SetVolumeAttachmentInfo(names.Tag, names.VolumeTag, state.VolumeAttachmentInfo) error

	CreateVolumeAttachmentPlan(names.Tag, names.VolumeTag, state.VolumeAttachmentPlanInfo) error
	RemoveVolumeAttachmentPlan(names.Tag, names.VolumeTag, bool) error
	SetVolumeAttachmentPlanBlockInfo(machineTag names.Tag, volumeTag names.VolumeTag, info state.BlockDeviceInfo) error
}

// TODO - CAAS(ericclaudejones): This should contain state alone, model will be
// removed once all relevant methods are moved from state to model.
type stateShim struct {
	*state.State
	*state.Model
}

// NewStateBackends creates a Backend from the given *state.State.
func NewStateBackends(st *state.State) (Backend, StorageBackend, error) {
	m, err := st.Model()
	if err != nil {
		return nil, nil, err
	}
	sb, err := state.NewStorageBackend(st)
	if err != nil {
		return nil, nil, err
	}
	return stateShim{State: st, Model: m}, sb, nil
}

func (s stateShim) WatchMachine(tag names.MachineTag) (state.NotifyWatcher, error) {
	m, err := s.Machine(tag.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return m.Watch(), nil
}
