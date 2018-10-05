// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"github.com/juju/cmd"
	"gopkg.in/juju/worker.v1/catacomb"

	"github.com/juju/juju/api"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
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
	command := &addCommand{
		api:               api,
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
func NewRemoveCommandForTest(apiRoot api.Connection, machineAPI RemoveMachineAPI) (cmd.Command, *RemoveCommand) {
	command := &removeCommand{
		apiRoot:    apiRoot,
		machineAPI: machineAPI,
	}
	command.SetClientStore(jujuclienttesting.MinimalStore())
	return modelcmd.Wrap(command), &RemoveCommand{command}
}

// NewUpgradeSeriesCommand returns an upgrade series command for test
func NewUpgradeSeriesCommandForTest(upgradeAPI UpgradeMachineSeriesAPI, leaderAPI leadership.Pinner) cmd.Command {
	command := &upgradeSeriesCommand{
		upgradeMachineSeriesClient: upgradeAPI,
		leadershipClient:           leaderAPI,
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
