// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"github.com/juju/cmd"

	"github.com/juju/juju/api"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/storage"
)

var (
	SSHProvisioner = &sshProvisioner
)

type AddCommand struct {
	*addCommand
}

// NewAddCommand returns an AddCommand with the api provided as specified.
func NewAddCommandForTest(api AddMachineAPI, mcAPI ModelConfigAPI, mmAPI MachineManagerAPI) (cmd.Command, *AddCommand) {
	cmd := &addCommand{
		api:               api,
		machineManagerAPI: mmAPI,
		modelConfigAPI:    mcAPI,
	}
	return modelcmd.Wrap(cmd), &AddCommand{cmd}
}

// NewListCommandForTest returns a listMachineCommand with specified api
func NewListCommandForTest(api statusAPI) cmd.Command {
	cmd := newListMachinesCommand(api)
	return modelcmd.Wrap(cmd)
}

// NewShowCommandForTest returns a showMachineCommand with specified api
func NewShowCommandForTest(api statusAPI) cmd.Command {
	cmd := newShowMachineCommand(api)
	return modelcmd.Wrap(cmd)
}

type RemoveCommand struct {
	*removeCommand
}

// NewRemoveCommand returns an RemoveCommand with the api provided as specified.
func NewRemoveCommandForTest(apiRoot api.Connection, machineAPI RemoveMachineAPI) (cmd.Command, *RemoveCommand) {
	cmd := &removeCommand{
		apiRoot:    apiRoot,
		machineAPI: machineAPI,
	}
	return modelcmd.Wrap(cmd), &RemoveCommand{cmd}
}

func NewDisksFlag(disks *[]storage.Constraints) *disksFlag {
	return &disksFlag{disks}
}
