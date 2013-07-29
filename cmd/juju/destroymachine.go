// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/names"
)

// DestroyMachineCommand causes an existing machine to be destroyed.
type DestroyMachineCommand struct {
	EnvCommandBase
	MachineIds []string
}

func (c *DestroyMachineCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "destroy-machine",
		Args:    "<machine> ...",
		Purpose: "destroy machines",
		Doc:     "Machines that have assigned units, or are responsible for the environment, cannot be destroyed.",
		Aliases: []string{"terminate-machine"},
	}
}

func (c *DestroyMachineCommand) Init(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no machines specified")
	}
	for _, id := range args {
		if !names.IsMachineId(id) {
			return fmt.Errorf("invalid machine id %q", id)
		}
	}
	c.MachineIds = args
	return nil
}

func (c *DestroyMachineCommand) Run(_ *cmd.Context) error {
	conn, err := juju.NewConnFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.State.DestroyMachines(c.MachineIds...)
}
