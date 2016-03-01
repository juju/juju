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
)

func NewPoolListCommandForTest(api PoolListAPI, store jujuclient.ClientStore) cmd.Command {
	cmd := &poolListCommand{newAPIFunc: func() (PoolListAPI, error) {
		return api, nil
	}}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}

func NewPoolCreateCommandForTest(api PoolCreateAPI, store jujuclient.ClientStore) cmd.Command {
	cmd := &poolCreateCommand{newAPIFunc: func() (PoolCreateAPI, error) {
		return api, nil
	}}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}

/*func NewVolumeListCommandForTest(api VolumeListAPI, store jujuclient.ClientStore) cmd.Command {
	cmd := &volumeListCommand{newAPIFunc: func() (VolumeListAPI, error) {
		return api, nil
	}}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}*/

func NewShowCommandForTest(api StorageShowAPI, store jujuclient.ClientStore) cmd.Command {
	cmd := &showCommand{newAPIFunc: func() (StorageShowAPI, error) {
		return api, nil
	}}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}

func NewListCommandForTest(api StorageListAPI, store jujuclient.ClientStore) cmd.Command {
	cmd := &listCommand{newAPIFunc: func() (StorageListAPI, error) {
		return api, nil
	}}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}

func NewAddCommandForTest(api StorageAddAPI, store jujuclient.ClientStore) cmd.Command {
	cmd := &addCommand{newAPIFunc: func() (StorageAddAPI, error) {
		return api, nil
	}}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}

/*func NewFilesystemListCommandForTest(api FilesystemListAPI, store jujuclient.ClientStore) cmd.Command {
	cmd := &filesystemListCommand{newAPIFunc: func() (FilesystemListAPI, error) {
		return api, nil
	}}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}*/
