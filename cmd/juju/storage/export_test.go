// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/cmd"

	"github.com/juju/juju/cmd/envcmd"
)

var (
	GetStorageShowAPI    = &getStorageShowAPI
	GetStorageListAPI    = &getStorageListAPI
	GetFilesystemListAPI = &getFilesystemListAPI

	ConvertToVolumeInfo     = convertToVolumeInfo
	ConvertToFilesystemInfo = convertToFilesystemInfo
	GetStorageAddAPI        = &getStorageAddAPI

	NewPoolSuperCommand   = newPoolSuperCommand
	NewVolumeSuperCommand = newVolumeSuperCommand
)

func NewPoolListCommand(api PoolListAPI) cmd.Command {
	cmd := &poolListCommand{api: api}
	return envcmd.Wrap(cmd)
}

func NewPoolCreateCommand(api PoolCreateAPI) cmd.Command {
	cmd := &poolCreateCommand{api: api}
	return envcmd.Wrap(cmd)
}

func NewVolumeListCommand(api VolumeListAPI) cmd.Command {
	cmd := &volumeListCommand{api: api}
	return envcmd.Wrap(cmd)
}
