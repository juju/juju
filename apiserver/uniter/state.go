// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/state"
)

type storageStateInterface interface {
	RemoveStorageAttachment(names.StorageTag, names.UnitTag) error
	StorageInstance(names.StorageTag) (state.StorageInstance, error)
	StorageInstanceFilesystem(names.StorageTag) (state.Filesystem, error)
	StorageInstanceVolume(names.StorageTag) (state.Volume, error)
	UnitStorageAttachments(names.UnitTag) ([]state.StorageAttachment, error)
	DestroyUnitStorageAttachments(names.UnitTag) error
	StorageAttachment(names.StorageTag, names.UnitTag) (state.StorageAttachment, error)
	UnitAssignedMachine(names.UnitTag) (names.MachineTag, error)
	FilesystemAttachment(names.MachineTag, names.FilesystemTag) (state.FilesystemAttachment, error)
	VolumeAttachment(names.MachineTag, names.VolumeTag) (state.VolumeAttachment, error)
	WatchStorageAttachments(names.UnitTag) state.StringsWatcher
	WatchStorageAttachment(names.StorageTag, names.UnitTag) state.NotifyWatcher
	WatchFilesystemAttachment(names.MachineTag, names.FilesystemTag) state.NotifyWatcher
	WatchVolumeAttachment(names.MachineTag, names.VolumeTag) state.NotifyWatcher
	AddStorageForUnit(tag names.UnitTag, name string, cons state.StorageConstraints) error
	UnitStorageConstraints(u names.UnitTag) (map[string]state.StorageConstraints, error)
}

type storageStateShim struct {
	*state.State
}

var getStorageState = func(st *state.State) storageStateInterface {
	return storageStateShim{st}
}

// UnitAssignedMachine returns the tag of the machine that the unit
// is assigned to, or an error if the unit cannot be obtained or is
// not assigned to a machine.
func (s storageStateShim) UnitAssignedMachine(tag names.UnitTag) (names.MachineTag, error) {
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

// UnitStorageConstraints returns storage constraints for this unit,
// or an error if the unit or its constraints cannot be obtained.
func (s storageStateShim) UnitStorageConstraints(u names.UnitTag) (map[string]state.StorageConstraints, error) {
	unit, err := s.Unit(u.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}

	cons, err := unit.StorageConstraints()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return cons, nil
}
