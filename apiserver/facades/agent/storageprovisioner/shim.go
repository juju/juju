// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
	"github.com/juju/juju/storage/poolmanager"
)

// This file contains untested shims to let us wrap state in a sensible
// interface and avoid writing tests that depend on mongodb. If you were
// to change any part of it so that it were no longer *obviously* and
// *trivially* correct, you would be Doing It Wrong.

// NewFacadeV3 provides the signature required for facade registration.
func NewFacadeV3(st *state.State, resources facade.Resources, authorizer facade.Authorizer) (*StorageProvisionerAPIv3, error) {
	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	registry, err := stateenvirons.NewStorageProviderRegistryForModel(
		model,
		stateenvirons.GetNewEnvironFunc(environs.New),
		stateenvirons.GetNewCAASBrokerFunc(caas.New))
	pm := poolmanager.New(state.NewStateSettings(st), registry)

	backend, storageBackend, err := NewStateBackends(st)
	if err != nil {
		return nil, errors.Annotate(err, "getting backend")
	}
	return NewStorageProvisionerAPIv3(backend, storageBackend, resources, authorizer, registry, pm)
}

// NewFacadeV4 provides the signature required for facade registration.
func NewFacadeV4(st *state.State, resources facade.Resources, authorizer facade.Authorizer) (*StorageProvisionerAPIv4, error) {
	v3, err := NewFacadeV3(st, resources, authorizer)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewStorageProvisionerAPIv4(v3), nil
}

type Backend interface {
	state.EntityFinder
	state.ModelAccessor

	ControllerConfig() (controller.Config, error)
	MachineInstanceId(names.MachineTag) (instance.Id, error)
	ModelTag() names.ModelTag
	WatchMachine(names.MachineTag) (state.NotifyWatcher, error)
	WatchApplications() state.StringsWatcher
}

type StorageBackend interface {
	BlockDevices(names.MachineTag) ([]state.BlockDeviceInfo, error)

	WatchBlockDevices(names.MachineTag) state.NotifyWatcher
	WatchModelFilesystems() state.StringsWatcher
	WatchModelFilesystemAttachments() state.StringsWatcher
	WatchMachineFilesystems(names.MachineTag) state.StringsWatcher
	WatchUnitFilesystems(tag names.ApplicationTag) state.StringsWatcher
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
	ReleaseStorageInstance(names.StorageTag, bool) error
	DetachStorage(names.StorageTag, names.UnitTag) error

	Filesystem(names.FilesystemTag) (state.Filesystem, error)
	FilesystemAttachment(names.Tag, names.FilesystemTag) (state.FilesystemAttachment, error)

	Volume(names.VolumeTag) (state.Volume, error)
	VolumeAttachment(names.Tag, names.VolumeTag) (state.VolumeAttachment, error)
	VolumeAttachments(names.VolumeTag) ([]state.VolumeAttachment, error)
	VolumeAttachmentPlan(names.Tag, names.VolumeTag) (state.VolumeAttachmentPlan, error)
	VolumeAttachmentPlans(volume names.VolumeTag) ([]state.VolumeAttachmentPlan, error)

	RemoveFilesystem(names.FilesystemTag) error
	RemoveFilesystemAttachment(names.Tag, names.FilesystemTag) error
	RemoveVolume(names.VolumeTag) error
	RemoveVolumeAttachment(names.Tag, names.VolumeTag) error
	DetachFilesystem(names.Tag, names.FilesystemTag) error
	DestroyFilesystem(names.FilesystemTag) error
	DetachVolume(names.Tag, names.VolumeTag) error
	DestroyVolume(names.VolumeTag) error

	SetFilesystemInfo(names.FilesystemTag, state.FilesystemInfo) error
	SetFilesystemAttachmentInfo(names.Tag, names.FilesystemTag, state.FilesystemAttachmentInfo) error
	SetVolumeInfo(names.VolumeTag, state.VolumeInfo) error
	SetVolumeAttachmentInfo(names.Tag, names.VolumeTag, state.VolumeAttachmentInfo) error

	CreateVolumeAttachmentPlan(names.Tag, names.VolumeTag, state.VolumeAttachmentPlanInfo) error
	RemoveVolumeAttachmentPlan(names.Tag, names.VolumeTag) error
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
