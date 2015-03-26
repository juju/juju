// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/state"
)

type storageAccess interface {
	/*
	   Required for instance functionality
	*/
	StorageInstance(names.StorageTag) (state.StorageInstance, error)
	AllStorageInstances() ([]state.StorageInstance, error)
	StorageAttachments(names.StorageTag) ([]state.StorageAttachment, error)
	UnitAssignedMachine(names.UnitTag) (names.MachineTag, error)
	FilesystemAttachment(names.MachineTag, names.FilesystemTag) (state.FilesystemAttachment, error)
	StorageInstanceFilesystem(names.StorageTag) (state.Filesystem, error)
	StorageInstanceVolume(names.StorageTag) (state.Volume, error)
	VolumeAttachment(names.MachineTag, names.VolumeTag) (state.VolumeAttachment, error)
	WatchFilesystemAttachment(names.MachineTag, names.FilesystemTag) state.NotifyWatcher
	WatchVolumeAttachment(names.MachineTag, names.VolumeTag) state.NotifyWatcher

	/*
	   Required for pool functionality
	*/
	EnvName() (string, error)

	/*
	   Required for volume functionality
	*/
	AllVolumes() ([]state.Volume, error)
	VolumeAttachments(volume names.VolumeTag) ([]state.VolumeAttachment, error)
	MachineVolumeAttachments(machine names.MachineTag) ([]state.VolumeAttachment, error)
	Volume(tag names.VolumeTag) (state.Volume, error)
}

var getState = func(st *state.State) storageAccess {
	return stateShim{st}
}

type stateShim struct {
	*state.State
}

// UnitAssignedMachine returns the tag of the machine that the unit
// is assigned to, or an error if the unit cannot be obtained or is
// not assigned to a machine.
func (s stateShim) UnitAssignedMachine(tag names.UnitTag) (names.MachineTag, error) {
	unit, err := s.Unit(tag.Id())
	if err != nil {
		return names.MachineTag{}, errors.Trace(err)
	}
	mid, err := unit.AssignedMachineId()
	if err != nil {
		return names.MachineTag{}, errors.Trace(err)
	}
	return names.NewMachineTag(mid), nil
}

// EnvName returns the name of Juju environment,
// or an error if environment configuration is not retrievable.
func (s stateShim) EnvName() (string, error) {
	cfg, err := s.State.EnvironConfig()
	if err != nil {
		return "", errors.Trace(err)
	}
	return cfg.Name(), nil
}
