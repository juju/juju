package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state"
)

// DestroyMachineCommand causes an existing machine to be destroyed.
type DestroyMachineCommand struct {
	EnvName    string
	MachineIds []string
}

func (c *DestroyMachineCommand) Info() *cmd.Info {
	return cmd.NewInfo(
		"destroy-machine", "<machine> [, ...]", "destroy machines",
		"Machines that have assigned units, or are responsible for the environment, cannot be destroyed.",
	)
}

func (c *DestroyMachineCommand) SetFlags(f *gnuflag.FlagSet) {
	addEnvironFlags(&c.EnvName, f)
}

func (c *DestroyMachineCommand) Init(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no machines specified")
	}
	for _, id := range args {
		if !state.IsMachineId(id) {
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
	return conn.DestroyMachines(c.MachineIds...)
}
