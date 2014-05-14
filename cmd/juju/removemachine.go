// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"

	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/cmd/envcmd"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/names"
)

// RemoveMachineCommand causes an existing machine to be destroyed.
type RemoveMachineCommand struct {
	envcmd.EnvCommandBase
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
	$ juju remove-machine 5

	# Remove machine 6 and any running units or containers
	$ juju remove-machine 6 --force
`

func (c *RemoveMachineCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "remove-machine",
		Args:    "<machine> ...",
		Purpose: "remove machines from the environment",
		Doc:     destroyMachineDoc,
		Aliases: []string{"destroy-machine", "terminate-machine"},
	}
}

func (c *RemoveMachineCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.Force, "force", false, "completely remove machine and all dependencies")
}

func (c *RemoveMachineCommand) Init(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no machines specified")
	}
	for _, id := range args {
		if !names.IsMachine(id) {
			return fmt.Errorf("invalid machine id %q", id)
		}
	}
	c.MachineIds = args
	return nil
}

func (c *RemoveMachineCommand) Run(_ *cmd.Context) error {
	apiclient, err := juju.NewAPIClientFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer apiclient.Close()
	if c.Force {
		return apiclient.ForceDestroyMachines(c.MachineIds...)
	}
	return apiclient.DestroyMachines(c.MachineIds...)
}
