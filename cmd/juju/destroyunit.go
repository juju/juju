package main

import (
	"errors"
	"fmt"

	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/trivial"
)

// DestroyUnitCommand is responsible for destroying service units.
type DestroyUnitCommand struct {
	EnvName   string
	UnitNames []string
}

func (c *DestroyUnitCommand) Info() *cmd.Info {
	return &cmd.Info{"destroy-unit", "<unit> [...]", "destroy service units", ""}
}

func (c *DestroyUnitCommand) Init(f *gnuflag.FlagSet, args []string) error {
	addEnvironFlags(&c.EnvName, f)
	if err := f.Parse(true, args); err != nil {
		return err
	}
	c.UnitNames = f.Args()
	if len(c.UnitNames) == 0 {
		return errors.New("no units specified")
	}
	for _, name := range c.UnitNames {
		if !state.IsUnitName(name) {
			return fmt.Errorf("invalid unit name: %q", name)
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
	defer trivial.ErrorContextf(&err, "cannot destroy units")
	var units []*state.Unit
	for _, name := range c.UnitNames {
		if unit, err := conn.State.Unit(name); state.IsNotFound(err) {
			return fmt.Errorf("unit %q is not alive", name)
		} else if err != nil {
			return err
		} else if unit.Life() != state.Alive {
			return fmt.Errorf("unit %q is not alive", name)
		} else if unit.IsPrincipal() {
			units = append(units, unit)
		} else {
			return fmt.Errorf("unit %q is a subordinate", name)
		}
	}
	return conn.DestroyUnits(units...)
}
