// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/cmd"

	"github.com/juju/juju/cmd/envcmd"
)

var (
	ConvertToVolumeInfo     = convertToVolumeInfo
	ConvertToFilesystemInfo = convertToFilesystemInfo

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

func NewShowCommand(api StorageShowAPI) cmd.Command {
	cmd := &showCommand{api: api}
	return envcmd.Wrap(cmd)
}

func NewListCommand(api StorageListAPI) cmd.Command {
	cmd := &listCommand{api: api}
	return envcmd.Wrap(cmd)
}

func NewAddCommand(api StorageAddAPI) cmd.Command {
	cmd := &addCommand{api: api}
	return envcmd.Wrap(cmd)
}

func NewFilesystemListCommand(api FilesystemListAPI) cmd.Command {
	cmd := &filesystemListCommand{api: api}
	return envcmd.Wrap(cmd)
}
