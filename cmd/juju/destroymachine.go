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

// DestroyMachineCommand causes an existing machine to be destroyed.
type DestroyMachineCommand struct {
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
	# Remove machine number 5, running no units or containers
	$ juju remove-machine 5

	# Remove machine 6, running units or containers
	$ juju remove-machine 6 --force
`

func (c *DestroyMachineCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "remove-machine",
		Args:    "<machine> ...",
		Purpose: "remove machines from the environment",
		Doc:     destroyMachineDoc,
		Aliases: []string{"destroy-machine", "terminate-machine"},
	}
}

func (c *DestroyMachineCommand) SetFlags(f *gnuflag.FlagSet) {
	c.EnvCommandBase.SetFlags(f)
	f.BoolVar(&c.Force, "force", false, "completely remove machine and all dependencies")
}

func (c *DestroyMachineCommand) Init(args []string) error {
	err := c.EnvCommandBase.Init()
	if err != nil {
		return err
	}
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

func (c *DestroyMachineCommand) Run(_ *cmd.Context) error {
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
