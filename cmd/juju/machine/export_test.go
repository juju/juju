// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"github.com/juju/cmd"

	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/storage"
)

var (
	ManualProvisioner = &manualProvisioner
)

type AddCommand struct {
	*addCommand
}

// NewAddCommand returns an AddCommand with the api provided as specified.
func NewAddCommandForTest(api AddMachineAPI, mmApi MachineManagerAPI) (cmd.Command, *AddCommand) {
	cmd := &addCommand{
		api:               api,
		machineManagerAPI: mmApi,
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
func NewRemoveCommandForTest(api RemoveMachineAPI) (cmd.Command, *RemoveCommand) {
	cmd := &removeCommand{
		api: api,
	}
	return modelcmd.Wrap(cmd), &RemoveCommand{cmd}
}

func NewDisksFlag(disks *[]storage.Constraints) *disksFlag {
	return &disksFlag{disks}
}
