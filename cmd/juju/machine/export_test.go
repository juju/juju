// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"github.com/juju/cmd/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/api"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
)

var (
	SSHProvisioner        = &sshProvisioner
	ErrDryRunNotSupported = errDryRunNotSupported
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

// NewUpgradeMachineCommand returns an upgrade machine command for test
func NewUpgradeMachineCommandForTest(statusAPI StatusAPI, upgradeAPI UpgradeMachineAPI) cmd.Command {
	command := &upgradeMachineCommand{
		statusClient:         statusAPI,
		upgradeMachineClient: upgradeAPI,
	}
	command.plan = catacomb.Plan{
		Site: &command.catacomb,
		Work: func() error { return nil },
	}
	command.SetClientStore(jujuclienttesting.MinimalStore())
	return modelcmd.Wrap(command)
}

func NewDisksFlag(disks *[]storage.Constraints) *disksFlag {
	return &disksFlag{disks}
}
