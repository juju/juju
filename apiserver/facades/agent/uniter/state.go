// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/core/blockdevice"
	coremodel "github.com/juju/juju/core/model"
	corewatcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/state"
)

type blockDeviceService interface {
	BlockDevices(ctx context.Context, machineId string) ([]blockdevice.BlockDevice, error)
	WatchBlockDevices(ctx context.Context, machineId string) (corewatcher.NotifyWatcher, error)
}

type storageAccess interface {
	storageInterface
	VolumeAccess() storageVolumeInterface
	FilesystemAccess() storageFilesystemInterface
}

type storageInterface interface {
	StorageInstance(names.StorageTag) (state.StorageInstance, error)
	UnitStorageAttachments(names.UnitTag) ([]state.StorageAttachment, error)
	RemoveStorageAttachment(names.StorageTag, names.UnitTag, bool) error
	DestroyUnitStorageAttachments(names.UnitTag) error
	StorageAttachment(names.StorageTag, names.UnitTag) (state.StorageAttachment, error)
	AddStorageForUnitOperation(names.UnitTag, string, state.StorageConstraints) (state.ModelOperation, error)
	WatchStorageAttachments(names.UnitTag) state.StringsWatcher
	WatchStorageAttachment(names.StorageTag, names.UnitTag) state.NotifyWatcher
}

type storageVolumeInterface interface {
	StorageInstanceVolume(names.StorageTag) (state.Volume, error)
	WatchVolumeAttachment(names.Tag, names.VolumeTag) state.NotifyWatcher
	VolumeAttachment(names.Tag, names.VolumeTag) (state.VolumeAttachment, error)
	VolumeAttachmentPlan(names.Tag, names.VolumeTag) (state.VolumeAttachmentPlan, error)
}

type storageFilesystemInterface interface {
	StorageInstanceFilesystem(names.StorageTag) (state.Filesystem, error)
	FilesystemAttachment(names.Tag, names.FilesystemTag) (state.FilesystemAttachment, error)
	WatchFilesystemAttachment(names.Tag, names.FilesystemTag) state.NotifyWatcher
}

var getStorageState = func(
	st *state.State,
	modelType coremodel.ModelType,
) (storageAccess, error) {
	sb, err := state.NewStorageConfigBackend(st)
	if err != nil {
		return nil, err
	}
	storageAccess := &storageShim{
		storageInterface: sb,
		va:               sb,
		fa:               sb,
	}
	// CAAS models don't support volume storage yet.
	if modelType == coremodel.CAAS {
		storageAccess.va = nil
	}
	return storageAccess, nil
}

type storageShim struct {
	storageInterface
	fa storageFilesystemInterface
	va storageVolumeInterface
}

func (s *storageShim) VolumeAccess() storageVolumeInterface {
	return s.va
}

func (s *storageShim) FilesystemAccess() storageFilesystemInterface {
	return s.fa
}

type backend interface {
	Unit(string) (Unit, error)
	ApplyOperation(state.ModelOperation) error
}

type Unit interface {
	AssignedMachineId() (string, error)
	ShouldBeAssigned() bool
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
