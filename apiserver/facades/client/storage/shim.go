// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common/storagecommon"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
	"github.com/juju/juju/storage/poolmanager"
)

// This file contains untested shims to let us wrap state in a sensible
// interface and avoid writing tests that depend on mongodb. If you were
// to change any part of it so that it were no longer *obviously* and
// *trivially* correct, you would be Doing It Wrong.

// NewFacadeV4 provides the signature required for facade registration.
func NewFacadeV4(
	st *state.State,
	resources facade.Resources,
	authorizer facade.Authorizer,
) (*APIv4, error) {
	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	registry, err := stateenvirons.NewStorageProviderRegistryForModel(
		model,
		stateenvirons.GetNewEnvironFunc(environs.New),
		stateenvirons.GetNewCAASBrokerFunc(caas.New))
	pm := poolmanager.New(state.NewStateSettings(st), registry)

	storageAccessor, err := getStorageAccessor(st)
	if err != nil {
		return nil, errors.Annotate(err, "getting backend")
	}
	return NewAPIv4(
		stateShim{st},
		storageAccessor,
		registry, pm, resources, authorizer,
		state.CallContext(st))
}

// NewFacadeV3 provides the signature required for facade registration.
func NewFacadeV3(
	st *state.State,
	resources facade.Resources,
	authorizer facade.Authorizer,
) (*APIv3, error) {
	env, err := stateenvirons.GetNewEnvironFunc(environs.New)(st)
	if err != nil {
		return nil, errors.Annotate(err, "getting environ")
	}
	registry := stateenvirons.NewStorageProviderRegistry(env)
	pm := poolmanager.New(state.NewStateSettings(st), registry)

	storageAccessor, err := getStorageAccessor(st)
	if err != nil {
		return nil, errors.Annotate(err, "getting backend")
	}
	return NewAPIv3(
		stateShim{st},
		storageAccessor,
		registry, pm, resources, authorizer,
		state.CallContext(st))
}

type storageAccess interface {
	storageInterface

	// VolumeAccess is required for storage functionality.
	VolumeAccess() storageVolume

	// FilesystemAccess is required for storage functionality.
	FilesystemAccess() storageFile
}

type storageInterface interface {
	// StorageInstance is required for storage functionality.
	StorageInstance(names.StorageTag) (state.StorageInstance, error)

	// AllStorageInstances is required for storage functionality.
	AllStorageInstances() ([]state.StorageInstance, error)

	// StorageAttachments is required for storage functionality.
	StorageAttachments(names.StorageTag) ([]state.StorageAttachment, error)

	// UnitStorageAttachments returns the storage attachments for the
	// identified unit.
	UnitStorageAttachments(names.UnitTag) ([]state.StorageAttachment, error)

	// AddStorageForUnit is required for storage add functionality.
	AddStorageForUnit(tag names.UnitTag, name string, cons state.StorageConstraints) ([]names.StorageTag, error)

	// AttachStorage attaches the storage instance with the
	// specified tag to the unit with the specified tag.
	AttachStorage(names.StorageTag, names.UnitTag) error

	// DetachStorage detaches the storage instance with the
	// specified tag from the unit with the specified tag.
	DetachStorage(names.StorageTag, names.UnitTag) error

	// DestroyStorageInstance destroys the storage instance with the specified tag.
	DestroyStorageInstance(names.StorageTag, bool) error

	// ReleaseStorageInstance releases the storage instance with the specified tag.
	ReleaseStorageInstance(names.StorageTag, bool) error
}

type storageVolume interface {
	storagecommon.VolumeAccess

	// AllVolumes is required for volume functionality.
	AllVolumes() ([]state.Volume, error)

	// VolumeAttachments is required for volume functionality.
	VolumeAttachments(volume names.VolumeTag) ([]state.VolumeAttachment, error)

	VolumeAttachmentPlans(volume names.VolumeTag) ([]state.VolumeAttachmentPlan, error)

	// MachineVolumeAttachments is required for volume functionality.
	MachineVolumeAttachments(machine names.MachineTag) ([]state.VolumeAttachment, error)

	// Volume is required for volume functionality.
	Volume(tag names.VolumeTag) (state.Volume, error)

	// AddExistingFilesystem imports an existing filesystem into the model.
	AddExistingFilesystem(f state.FilesystemInfo, v *state.VolumeInfo, storageName string) (names.StorageTag, error)
}

type storageFile interface {
	storagecommon.FilesystemAccess

	// AllFilesystems is required for filesystem functionality.
	AllFilesystems() ([]state.Filesystem, error)

	// FilesystemAttachments is required for filesystem functionality.
	FilesystemAttachments(filesystem names.FilesystemTag) ([]state.FilesystemAttachment, error)

	// MachineFilesystemAttachments is required for filesystem functionality.
	MachineFilesystemAttachments(machine names.MachineTag) ([]state.FilesystemAttachment, error)

	// Filesystem is required for filesystem functionality.
	Filesystem(tag names.FilesystemTag) (state.Filesystem, error)

	// AddExistingFilesystem imports an existing filesystem into the model.
	AddExistingFilesystem(f state.FilesystemInfo, v *state.VolumeInfo, storageName string) (names.StorageTag, error)
}

var getStorageAccessor = func(st *state.State) (storageAccess, error) {
	sb, err := state.NewStorageBackend(st)
	if err != nil {
		return nil, err
	}
	storageAccess := &storageShim{
		storageInterface: sb,
		va:               sb,
		fa:               sb,
	}
	return storageAccess, nil
}

type storageShim struct {
	storageInterface
	fa storageFile
	va storageVolume
}

func (s *storageShim) VolumeAccess() storageVolume {
	return s.va
}

func (s *storageShim) FilesystemAccess() storageFile {
	return s.fa
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

type backend interface {
	ControllerTag() names.ControllerTag
	ModelTag() names.ModelTag
	Unit(string) (Unit, error)
	GetBlockForType(state.BlockType) (state.Block, bool, error)
}

type Unit interface {
	AssignedMachineId() (string, error)
}

type stateShim struct {
	*state.State
}

func (s stateShim) ModelTag() names.ModelTag {
	return names.NewModelTag(s.ModelUUID())
}

func (s stateShim) GetBlockForType(t state.BlockType) (state.Block, bool, error) {
	return s.State.GetBlockForType(t)
}

func (s stateShim) Unit(name string) (Unit, error) {
	return s.State.Unit(name)
}
