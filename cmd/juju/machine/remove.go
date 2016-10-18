// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
)

// NewRemoveCommand returns a command used to remove a specified machine.
func NewRemoveCommand() cmd.Command {
	return modelcmd.Wrap(&removeCommand{})
}

// removeCommand causes an existing machine to be destroyed.
type removeCommand struct {
	modelcmd.ModelCommandBase
	api        RemoveMachineAPI
	MachineIds []string
	Force      bool
}

const destroyMachineDoc = `
Machines are specified by their numbers, which may be retrieved from the
output of ` + "`juju status`." + `
Machines responsible for the model cannot be removed.
Machines running units or containers can be removed using the '--force'
option; this will also remove those units and containers without giving
them an opportunity to shut down cleanly.

Examples:

Remove machine number 5 which has no running units or containers:

    juju remove-machine 5

Remove machine 6 and any running units or containers:

    juju remove-machine 6 --force

See also:
    add-machine
`

// Info implements Command.Info.
func (c *removeCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "remove-machine",
		Args:    "<machine number> ...",
		Purpose: "Removes one or more machines from a model.",
		Doc:     destroyMachineDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *removeCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.BoolVar(&c.Force, "force", false, "Completely remove a machine and all its dependencies")
}

func (c *removeCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.Errorf("no machines specified")
	}
	for _, id := range args {
		if !names.IsValidMachine(id) {
			return errors.Errorf("invalid machine id %q", id)
		}
	}
	c.MachineIds = args
	return nil
}

type RemoveMachineAPI interface {
	DestroyMachines(machines ...string) error
	ForceDestroyMachines(machines ...string) error
	Close() error
}

func (c *removeCommand) getRemoveMachineAPI() (RemoveMachineAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	return c.NewAPIClient()
}

// Run implements Command.Run.
func (c *removeCommand) Run(_ *cmd.Context) error {
	client, err := c.getRemoveMachineAPI()
	if err != nil {
		return err
	}
	defer client.Close()
	if c.Force {
		err = client.ForceDestroyMachines(c.MachineIds...)
	} else {
		err = client.DestroyMachines(c.MachineIds...)
	}
	return block.ProcessBlockedError(err, block.BlockRemove)
}
