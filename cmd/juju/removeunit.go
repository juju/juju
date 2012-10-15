package main

import (
	"errors"
	"fmt"

	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state"
)

// RemoveUnitCommand is responsible removing service units.
type RemoveUnitCommand struct {
	EnvName   string
	UnitNames []string
}

func (c *RemoveUnitCommand) Info() *cmd.Info {
	return &cmd.Info{"remove-unit", "<unit> [...]", "remove service units", ""}
}

func (c *RemoveUnitCommand) Init(f *gnuflag.FlagSet, args []string) error {
	addEnvironFlags(&c.EnvName, f)
	if err := f.Parse(true, args); err != nil {
		return err
	}
	c.UnitNames = f.Args()
	if len(c.UnitNames) == 0 {
		return errors.New("no service units specified")
	}
	for _, name := range c.UnitNames {
		if !state.IsUnitName(name) {
			return fmt.Errorf("invalid service unit name: %q", name)
		}
	}
	return nil
}

// Run connects to the environment specified on the command line 
// and calls conn.RemoveUnits.
func (c *RemoveUnitCommand) Run(_ *cmd.Context) error {
	conn, err := juju.NewConnFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer conn.Close()
	var units []*state.Unit
	for _, name := range c.UnitNames {
		unit, err := conn.State.Unit(name)
		if err != nil {
			return err
		}
		units = append(units, unit)
	}
	return conn.RemoveUnits(units...)
}
