// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/state"
)

type storageInterface interface {
	StorageInstance(names.StorageTag) (state.StorageInstance, error)
	UnitStorageAttachments(names.UnitTag) ([]state.StorageAttachment, error)
	RemoveStorageAttachment(names.StorageTag, names.UnitTag) error
	DestroyUnitStorageAttachments(names.UnitTag) error
	StorageAttachment(names.StorageTag, names.UnitTag) (state.StorageAttachment, error)
	AddStorageForUnit(tag names.UnitTag, name string, cons state.StorageConstraints) ([]names.StorageTag, error)
	WatchStorageAttachments(names.UnitTag) state.StringsWatcher
	WatchStorageAttachment(names.StorageTag, names.UnitTag) state.NotifyWatcher
}

type storageVolumeInterface interface {
	StorageInstanceVolume(names.StorageTag) (state.Volume, error)
	BlockDevices(names.MachineTag) ([]state.BlockDeviceInfo, error)
	WatchVolumeAttachment(names.MachineTag, names.VolumeTag) state.NotifyWatcher
	WatchBlockDevices(names.MachineTag) state.NotifyWatcher
	VolumeAttachment(names.MachineTag, names.VolumeTag) (state.VolumeAttachment, error)
}

type storageFilesystemInterface interface {
	StorageInstanceFilesystem(names.StorageTag) (state.Filesystem, error)
	FilesystemAttachment(names.MachineTag, names.FilesystemTag) (state.FilesystemAttachment, error)
	WatchFilesystemAttachment(names.MachineTag, names.FilesystemTag) state.NotifyWatcher
}

var getStorageState = func(st *state.State) (storageInterface, storageVolumeInterface, storageFilesystemInterface, error) {
	m, err := st.Model()
	if err != nil {
		return nil, nil, nil, err
	}
	if m.Type() == state.ModelTypeIAAS {
		im, _ := m.IAASModel()
		storageAccess := &iaasModelShim{Model: m, IAASModel: im}
		return im, storageAccess, storageAccess, nil
	}
	caasModel, _ := m.CAASModel()
	storageAccess := caasModelShim{Model: m, CAASModel: caasModel}
	// CAAS models don't support volume storage yet.
	return caasModel, nil, storageAccess, nil
}

type iaasModelShim struct {
	*state.Model
	*state.IAASModel
}

type caasModelShim struct {
	*state.Model
	*state.CAASModel
}

type backend interface {
	Unit(string) (Unit, error)
}

type Unit interface {
	AssignedMachineId() (string, error)
	StorageConstraints() (map[string]state.StorageConstraints, error)
}

type stateShim struct {
	*state.State
}

func (s stateShim) Unit(name string) (Unit, error) {
	return s.State.Unit(name)
}

// unitAssignedMachine returns the tag of the machine that the unit
// is assigned to, or an error if the unit cannot be obtained or is
// not assigned to a machine.
func unitAssignedMachine(backend backend, tag names.UnitTag) (names.MachineTag, error) {
	unit, err := backend.Unit(tag.Id())
	if err != nil {
		return names.MachineTag{}, errors.Trace(err)
	}
	mid, err := unit.AssignedMachineId()
	if err != nil {
		return names.MachineTag{}, errors.Trace(err)
	}
	return names.NewMachineTag(mid), nil
}

// unitStorageConstraints returns storage constraints for this unit,
// or an error if the unit or its constraints cannot be obtained.
func unitStorageConstraints(backend backend, u names.UnitTag) (map[string]state.StorageConstraints, error) {
	unit, err := backend.Unit(u.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}

	cons, err := unit.StorageConstraints()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return cons, nil
}
