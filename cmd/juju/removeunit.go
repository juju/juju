package main

import (
	"errors"

	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state"
)

// RemoveUnitCommand is responsible adding additional units to a service.
type RemoveUnitCommand struct {
	EnvName   string
	UnitNames []string
}

func (c *RemoveUnitCommand) Info() *cmd.Info {
	return &cmd.Info{"remove-unit", "", "removes service units (service/0, service/1, etc)", ""}
}

func (c *RemoveUnitCommand) Init(f *gnuflag.FlagSet, args []string) error {
	addEnvironFlags(&c.EnvName, f)
	if err := f.Parse(true, args); err != nil {
		return err
	}
	args = f.Args()
	if len(args) == 0 {
		return errors.New("no service units specified")
	}
	c.UnitNames = f.Args()
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
