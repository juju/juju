// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/cmd"

	"github.com/juju/juju/cmd/modelcmd"
)

var (
	ConvertToVolumeInfo     = convertToVolumeInfo
	ConvertToFilesystemInfo = convertToFilesystemInfo
)

func NewPoolListCommandForTest(api PoolListAPI) cmd.Command {
	cmd := &poolListCommand{newAPIFunc: func() (PoolListAPI, error) {
		return api, nil
	}}
	return modelcmd.Wrap(cmd)
}

func NewPoolCreateCommandForTest(api PoolCreateAPI) cmd.Command {
	cmd := &poolCreateCommand{newAPIFunc: func() (PoolCreateAPI, error) {
		return api, nil
	}}
	return modelcmd.Wrap(cmd)
}

/*func NewVolumeListCommandForTest(api VolumeListAPI) cmd.Command {
	cmd := &volumeListCommand{newAPIFunc: func() (VolumeListAPI, error) {
		return api, nil
	}}
	return modelcmd.Wrap(cmd)
}*/

func NewShowCommandForTest(api StorageShowAPI) cmd.Command {
	cmd := &showCommand{newAPIFunc: func() (StorageShowAPI, error) {
		return api, nil
	}}
	return modelcmd.Wrap(cmd)
}

func NewListCommandForTest(api StorageListAPI) cmd.Command {
	cmd := &listCommand{newAPIFunc: func() (StorageListAPI, error) {
		return api, nil
	}}
	return modelcmd.Wrap(cmd)
}

func NewAddCommandForTest(api StorageAddAPI) cmd.Command {
	cmd := &addCommand{newAPIFunc: func() (StorageAddAPI, error) {
		return api, nil
	}}
	return modelcmd.Wrap(cmd)
}

/*func NewFilesystemListCommandForTest(api StorageListAPI) cmd.Command {
	cmd := &listCommand{newAPIFunc: func() (StorageListAPI, error) {
		return api, nil
	}}
	return modelcmd.Wrap(cmd)
}*/
