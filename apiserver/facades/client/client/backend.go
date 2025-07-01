// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common/storagecommon"
	"github.com/juju/juju/state"
)

// Backend contains the state.State methods used in this package,
// allowing stubs to be created for testing.
type Backend interface {
	AllMachines() ([]*state.Machine, error)
	AllStatus() (*state.AllStatus, error)
}

type stateShim struct {
	*state.State
}

type StorageInterface interface {
	storagecommon.StorageAccess
	storagecommon.VolumeAccess
	storagecommon.FilesystemAccess

	AllStorageInstances() ([]state.StorageInstance, error)
	AllFilesystems() ([]state.Filesystem, error)
	AllVolumes() ([]state.Volume, error)

	StorageAttachments(names.StorageTag) ([]state.StorageAttachment, error)
	FilesystemAttachments(names.FilesystemTag) ([]state.FilesystemAttachment, error)
	VolumeAttachments(names.VolumeTag) ([]state.VolumeAttachment, error)
}

var getStorageState = func(st *state.State) (StorageInterface, error) {
	sb, err := state.NewStorageBackend(st)
	if err != nil {
		return nil, err
	}
	return sb, nil
}
