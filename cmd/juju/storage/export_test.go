// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"context"

	"github.com/juju/juju/api/jujuclient"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/internal/cmd"
)

var (
	ConvertToVolumeInfo     = convertToVolumeInfo
	ConvertToFilesystemInfo = convertToFilesystemInfo
)

func NewPoolListCommandForTest(api PoolListAPI, store jujuclient.ClientStore) cmd.Command {
	cmd := &poolListCommand{newAPIFunc: func(ctx context.Context) (PoolListAPI, error) {
		return api, nil
	}}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}

func NewPoolCreateCommandForTest(api PoolCreateAPI, store jujuclient.ClientStore) cmd.Command {
	cmd := &poolCreateCommand{newAPIFunc: func(ctx context.Context) (PoolCreateAPI, error) {
		return api, nil
	}}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}

func NewPoolRemoveCommandForTest(api PoolRemoveAPI, store jujuclient.ClientStore) cmd.Command {
	cmd := &poolRemoveCommand{newAPIFunc: func(ctx context.Context) (PoolRemoveAPI, error) {
		return api, nil
	}}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}

func NewPoolUpdateCommandForTest(api PoolUpdateAPI, store jujuclient.ClientStore) cmd.Command {
	cmd := &poolUpdateCommand{newAPIFunc: func(ctx context.Context) (PoolUpdateAPI, error) {
		return api, nil
	}}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}

func NewShowCommandForTest(api StorageShowAPI, store jujuclient.ClientStore) cmd.Command {
	cmd := &showCommand{newAPIFunc: func(ctx context.Context) (StorageShowAPI, error) {
		return api, nil
	}}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}

func NewListCommandForTest(api StorageListAPI, store jujuclient.ClientStore) cmd.Command {
	cmd := &listCommand{newAPIFunc: func(ctx context.Context) (StorageListAPI, error) {
		return api, nil
	}}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}

func NewAddCommandForTest(api StorageAddAPI, store jujuclient.ClientStore) cmd.Command {
	cmd := &addCommand{newAPIFunc: func(ctx context.Context) (StorageAddAPI, error) {
		return api, nil
	}}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}

func NewRemoveStorageCommandForTest(new NewStorageRemoverCloserFunc, store jujuclient.ClientStore) cmd.Command {
	cmd := &removeStorageCommand{}
	cmd.SetClientStore(store)
	cmd.newStorageRemoverCloser = new
	return modelcmd.Wrap(cmd)
}

func NewAttachStorageCommandForTest(new NewEntityAttacherCloserFunc, store jujuclient.ClientStore) cmd.Command {
	cmd := &attachStorageCommand{}
	cmd.SetClientStore(store)
	cmd.newEntityAttacherCloser = new
	return modelcmd.Wrap(cmd)
}

func NewDetachStorageCommandForTest(new NewEntityDetacherCloserFunc, store jujuclient.ClientStore) cmd.Command {
	cmd := &detachStorageCommand{}
	cmd.SetClientStore(store)
	cmd.newEntityDetacherCloser = new
	return modelcmd.Wrap(cmd)
}
