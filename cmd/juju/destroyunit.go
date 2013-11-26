// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/rpc"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/names"
)

// DestroyUnitCommand is responsible for destroying service units.
type DestroyUnitCommand struct {
	cmd.EnvCommandBase
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
		return fmt.Errorf("no units specified")
	}
	for _, name := range c.UnitNames {
		if !names.IsUnit(name) {
			return fmt.Errorf("invalid unit name %q", name)
		}
	}
	return nil
}

// destroyUnits destroys the units with the specified names.
// This is copied from the 1.16.3 code to enable compatibility. It should be
// removed when we release a version that goes via the API only (whatever is
// after 1.18)
func destroyUnits(st *state.State, names ...string) (err error) {
	var errs []string
	for _, name := range names {
		unit, err := st.Unit(name)
		switch {
		case errors.IsNotFoundError(err):
			err = fmt.Errorf("unit %q does not exist", name)
		case err != nil:
		case unit.Life() != state.Alive:
			continue
		case unit.IsPrincipal():
			err = unit.Destroy()
		default:
			err = fmt.Errorf("unit %q is a subordinate", name)
		}
		if err != nil {
			errs = append(errs, err.Error())
		}
	}
	// destroyErr is shared with destroymachine.go, but will also be
	// removed when 1.16.3 compat is removed.
	return destroyErr("units", names, errs)
}

func (c *DestroyUnitCommand) run1dot16() error {
	conn, err := juju.NewConnFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer conn.Close()
	return destroyUnits(conn.State, c.UnitNames...)
}

// Run connects to the environment specified on the command line and destroys
// units therein.
func (c *DestroyUnitCommand) Run(_ *cmd.Context) error {
	client, err := juju.NewAPIClientFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer client.Close()
	err = client.DestroyServiceUnits(c.UnitNames...)
	// Juju 1.16.3 and older did not have DestroyMachines as an API command.
	if rpc.IsNoSuchRequest(err) {
		return c.run1dot16()
	}
	return err
}
