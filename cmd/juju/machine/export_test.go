// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"github.com/juju/juju/api"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
)

var (
	SSHProvisioner = &sshProvisioner
)

type AddCommand struct {
	*addCommand
}

// NewAddCommand returns an AddCommand with the api provided as specified.
func NewAddCommandForTest(mcAPI ModelConfigAPI, mmAPI MachineManagerAPI) (cmd.Command, *AddCommand) {
	command := &addCommand{
		machineManagerAPI: mmAPI,
		modelConfigAPI:    mcAPI,
	}
	command.SetClientStore(jujuclienttesting.MinimalStore())
	return modelcmd.Wrap(command), &AddCommand{command}
}

// NewListCommandForTest returns a listMachineCommand with specified api
func NewListCommandForTest(api statusAPI) cmd.Command {
	command := newListMachinesCommand(api)
	command.SetClientStore(jujuclienttesting.MinimalStore())
	return modelcmd.Wrap(command)
}

// NewShowCommandForTest returns a showMachineCommand with specified api
func NewShowCommandForTest(api statusAPI) cmd.Command {
	command := newShowMachineCommand(api)
	command.SetClientStore(jujuclienttesting.MinimalStore())
	return modelcmd.Wrap(command)
}

type RemoveCommand struct {
	*removeCommand
}

// NewRemoveCommand returns an RemoveCommand with the api provided as specified.
func NewRemoveCommandForTest(apiRoot api.Connection, machineAPI RemoveMachineAPI, modelConfigApi ModelConfigAPI) (cmd.Command, *RemoveCommand) {
	command := &removeCommand{
		apiRoot:        apiRoot,
		machineAPI:     machineAPI,
		modelConfigApi: modelConfigApi,
	}
	command.SetClientStore(jujuclienttesting.MinimalStore())
	return modelcmd.Wrap(command), &RemoveCommand{command}
}

func NewDisksFlag(disks *[]storage.Directive) *disksFlag {
	return &disksFlag{disks}
}
