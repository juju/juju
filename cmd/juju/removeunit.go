// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"errors"
	"fmt"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/cmd/envcmd"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/names"
)

// RemoveUnitCommand is responsible for destroying service units.
type RemoveUnitCommand struct {
	envcmd.EnvCommandBase
	UnitNames []string
}

func (c *RemoveUnitCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "remove-unit",
		Args:    "<unit> [...]",
		Purpose: "remove service units from the environment",
		Aliases: []string{"destroy-unit"},
	}
}

func (c *RemoveUnitCommand) Init(args []string) error {
	c.UnitNames = args
	if len(c.UnitNames) == 0 {
		return errors.New("no units specified")
	}
	for _, name := range c.UnitNames {
		if !names.IsUnit(name) {
			return fmt.Errorf("invalid unit name %q", name)
		}
	}
	return nil
}

// Run connects to the environment specified on the command line and destroys
// units therein.
func (c *RemoveUnitCommand) Run(_ *cmd.Context) error {
	client, err := juju.NewAPIClientFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer client.Close()
	return client.DestroyServiceUnits(c.UnitNames...)
}
