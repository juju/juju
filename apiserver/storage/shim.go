// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
	"github.com/juju/juju/storage/poolmanager"
)

// This file contains untested shims to let us wrap state in a sensible
// interface and avoid writing tests that depend on mongodb. If you were
// to change any part of it so that it were no longer *obviously* and
// *trivially* correct, you would be Doing It Wrong.

func init() {
	common.RegisterStandardFacade("Storage", 3, newAPI)
}

func newAPI(
	st *state.State,
	resources facade.Resources,
	authorizer facade.Authorizer,
) (*API, error) {
	env, err := stateenvirons.GetNewEnvironFunc(environs.New)(st)
	if err != nil {
		return nil, errors.Annotate(err, "getting environ")
	}
	registry := stateenvirons.NewStorageProviderRegistry(env)
	pm := poolmanager.New(state.NewStateSettings(st), registry)
	return NewAPI(getState(st), registry, pm, resources, authorizer)
}

type storageAccess interface {
	// StorageInstance is required for storage functionality.
	StorageInstance(names.StorageTag) (state.StorageInstance, error)

	// AllStorageInstances is required for storage functionality.
	AllStorageInstances() ([]state.StorageInstance, error)

	// StorageAttachments is required for storage functionality.
	StorageAttachments(names.StorageTag) ([]state.StorageAttachment, error)

	// UnitAssignedMachine is required for storage functionality.
	UnitAssignedMachine(names.UnitTag) (names.MachineTag, error)

	// FilesystemAttachment is required for storage functionality.
	FilesystemAttachment(names.MachineTag, names.FilesystemTag) (state.FilesystemAttachment, error)

	// StorageInstanceFilesystem is required for storage functionality.
	StorageInstanceFilesystem(names.StorageTag) (state.Filesystem, error)

	// StorageInstanceVolume is required for storage functionality.
	StorageInstanceVolume(names.StorageTag) (state.Volume, error)

	// VolumeAttachment is required for storage functionality.
	VolumeAttachment(names.MachineTag, names.VolumeTag) (state.VolumeAttachment, error)

	// WatchStorageAttachment is required for storage functionality.
	WatchStorageAttachment(names.StorageTag, names.UnitTag) state.NotifyWatcher

	// WatchFilesystemAttachment is required for storage functionality.
	WatchFilesystemAttachment(names.MachineTag, names.FilesystemTag) state.NotifyWatcher

	// WatchVolumeAttachment is required for storage functionality.
	WatchVolumeAttachment(names.MachineTag, names.VolumeTag) state.NotifyWatcher

	// WatchBlockDevices is required for storage functionality.
	WatchBlockDevices(names.MachineTag) state.NotifyWatcher

	// BlockDevices is required for storage functionality.
	BlockDevices(names.MachineTag) ([]state.BlockDeviceInfo, error)

	// ModelName is required for pool functionality.
	ModelName() (string, error)

	// ModelTag is required for model permission checking.
	ModelTag() names.ModelTag

	// AllVolumes is required for volume functionality.
	AllVolumes() ([]state.Volume, error)

	// VolumeAttachments is required for volume functionality.
	VolumeAttachments(volume names.VolumeTag) ([]state.VolumeAttachment, error)

	// MachineVolumeAttachments is required for volume functionality.
	MachineVolumeAttachments(machine names.MachineTag) ([]state.VolumeAttachment, error)

	// Volume is required for volume functionality.
	Volume(tag names.VolumeTag) (state.Volume, error)

	// AllFilesystems is required for filesystem functionality.
	AllFilesystems() ([]state.Filesystem, error)

	// FilesystemAttachments is required for filesystem functionality.
	FilesystemAttachments(filesystem names.FilesystemTag) ([]state.FilesystemAttachment, error)

	// MachineFilesystemAttachments is required for filesystem functionality.
	MachineFilesystemAttachments(machine names.MachineTag) ([]state.FilesystemAttachment, error)

	// Filesystem is required for filesystem functionality.
	Filesystem(tag names.FilesystemTag) (state.Filesystem, error)

	// AddStorageForUnit is required for storage add functionality.
	AddStorageForUnit(tag names.UnitTag, name string, cons state.StorageConstraints) error

	// GetBlockForType is required to block operations.
	GetBlockForType(t state.BlockType) (state.Block, bool, error)
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

// ModelName returns the name of Juju environment,
// or an error if environment configuration is not retrievable.
func (s stateShim) ModelName() (string, error) {
	cfg, err := s.State.ModelConfig()
	if err != nil {
		return "", errors.Trace(err)
	}
	return cfg.Name(), nil
}
