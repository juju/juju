package main

import (
	"errors"
	"fmt"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state"
)

// DestroyUnitCommand is responsible for destroying service units.
type DestroyUnitCommand struct {
	EnvCommandBase
	UnitNames []string
}

func (c *DestroyUnitCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "destroy-unit",
		Args:    "<unit> [...]",
		Purpose: "destroy service units",
		Aliases: []string{"remove-unit"},
	}
}

func (c *DestroyUnitCommand) Init(args []string) error {
	c.UnitNames = args
	if len(c.UnitNames) == 0 {
		return errors.New("no units specified")
	}
	for _, name := range c.UnitNames {
		if !state.IsUnitName(name) {
			return fmt.Errorf("invalid unit name %q", name)
		}
	}
	return nil
}

// Run connects to the environment specified on the command line
// and calls conn.DestroyUnits.
func (c *DestroyUnitCommand) Run(_ *cmd.Context) (err error) {
	conn, err := juju.NewConnFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.DestroyUnits(c.UnitNames...)
}
