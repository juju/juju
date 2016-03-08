// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/cmd"

	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

var (
	ConvertToVolumeInfo     = convertToVolumeInfo
	ConvertToFilesystemInfo = convertToFilesystemInfo

	NewPoolSuperCommand   = newPoolSuperCommand
	NewVolumeSuperCommand = newVolumeSuperCommand
)

func NewPoolListCommand(api PoolListAPI, store jujuclient.ClientStore) cmd.Command {
	cmd := &poolListCommand{newAPIFunc: func() (PoolListAPI, error) {
		return api, nil
	}}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}

func NewPoolCreateCommand(api PoolCreateAPI, store jujuclient.ClientStore) cmd.Command {
	cmd := &poolCreateCommand{newAPIFunc: func() (PoolCreateAPI, error) {
		return api, nil
	}}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}

func NewVolumeListCommand(api VolumeListAPI, store jujuclient.ClientStore) cmd.Command {
	cmd := &volumeListCommand{newAPIFunc: func() (VolumeListAPI, error) {
		return api, nil
	}}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}

func NewShowCommand(api StorageShowAPI, store jujuclient.ClientStore) cmd.Command {
	cmd := &showCommand{newAPIFunc: func() (StorageShowAPI, error) {
		return api, nil
	}}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}

func NewListCommand(api StorageListAPI, store jujuclient.ClientStore) cmd.Command {
	cmd := &listCommand{newAPIFunc: func() (StorageListAPI, error) {
		return api, nil
	}}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}

func NewAddCommand(api StorageAddAPI, store jujuclient.ClientStore) cmd.Command {
	cmd := &addCommand{newAPIFunc: func() (StorageAddAPI, error) {
		return api, nil
	}}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}

func NewFilesystemListCommand(api FilesystemListAPI, store jujuclient.ClientStore) cmd.Command {
	cmd := &filesystemListCommand{newAPIFunc: func() (FilesystemListAPI, error) {
		return api, nil
	}}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}
