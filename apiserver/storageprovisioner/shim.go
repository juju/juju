// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
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

func init() {
	common.RegisterStandardFacade("StorageProvisioner", 3, newStorageProvisionerAPI)
}

func newStorageProvisionerAPI(st *state.State, resources facade.Resources, authorizer facade.Authorizer) (*StorageProvisionerAPI, error) {
	env, err := stateenvirons.GetNewEnvironFunc(environs.New)(st)
	if err != nil {
		return nil, errors.Annotate(err, "getting environ")
	}
	registry := stateenvirons.NewStorageProviderRegistry(env)
	pm := poolmanager.New(state.NewStateSettings(st), registry)
	return NewStorageProvisionerAPI(stateShim{st}, resources, authorizer, registry, pm)
}

type Backend interface {
	state.EntityFinder
	state.ModelAccessor

	ControllerConfig() (controller.Config, error)
	MachineInstanceId(names.MachineTag) (instance.Id, error)
	ModelTag() names.ModelTag
	BlockDevices(names.MachineTag) ([]state.BlockDeviceInfo, error)

	WatchBlockDevices(names.MachineTag) state.NotifyWatcher
	WatchMachine(names.MachineTag) (state.NotifyWatcher, error)
	WatchModelFilesystems() state.StringsWatcher
	WatchEnvironFilesystemAttachments() state.StringsWatcher
	WatchMachineFilesystems(names.MachineTag) state.StringsWatcher
	WatchMachineFilesystemAttachments(names.MachineTag) state.StringsWatcher
	WatchModelVolumes() state.StringsWatcher
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

// NewStateBackend creates a Backend from the given *state.State.
func NewStateBackend(st *state.State) Backend {
	return stateShim{st}
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
