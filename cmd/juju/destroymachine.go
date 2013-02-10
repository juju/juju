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
	return &cmd.Info{
		"destroy-machine", "<machine> [, ...]", "destroy machines",
		"Machines that have assigned units, or are responsible for the environment, cannot be destroyed.",
	}
}

func (c *DestroyMachineCommand) Init(f *gnuflag.FlagSet, args []string) error {
	addEnvironFlags(&c.EnvName, f)
	if err := f.Parse(true, args); err != nil {
		return err
	}
	args = f.Args()
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
