// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/names"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/block"
)

// RemoveCommand causes an existing machine to be destroyed.
type RemoveCommand struct {
	envcmd.EnvCommandBase
	api        RemoveMachineAPI
	MachineIds []string
	Force      bool
}

const destroyMachineDoc = `
Machines that are responsible for the environment cannot be removed. Machines
running units or containers can only be removed with the --force flag; doing
so will also remove all those units and containers without giving them any
opportunity to shut down cleanly.

Examples:
	# Remove machine number 5 which has no running units or containers
	$ juju machine remove 5

	# Remove machine 6 and any running units or containers
	$ juju machine remove 6 --force
`

func (c *RemoveCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "remove",
		Args:    "<machine> ...",
		Purpose: "remove machines from the environment",
		Doc:     destroyMachineDoc,
	}
}

func (c *RemoveCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.Force, "force", false, "completely remove machine and all dependencies")
}

func (c *RemoveCommand) Init(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no machines specified")
	}
	for _, id := range args {
		if !names.IsValidMachine(id) {
			return fmt.Errorf("invalid machine id %q", id)
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

func (c *RemoveCommand) getRemoveMachineAPI() (RemoveMachineAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	return c.NewAPIClient()
}

func (c *RemoveCommand) Run(_ *cmd.Context) error {
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
