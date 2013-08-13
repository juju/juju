// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"errors"
	"fmt"

	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/statecmd"
)

// UnitCommandBase provides support for commands which deploy units. It handles the parsing
// and validation of --to and --num-units arguments.
type UnitCommandBase struct {
	ToMachineSpec string
	NumUnits      int
}

func (c *UnitCommandBase) SetFlags(f *gnuflag.FlagSet) {
	f.IntVar(&c.NumUnits, "num-units", 1, "")
	f.StringVar(&c.ToMachineSpec, "to", "", "the machine or container to deploy the unit in, bypasses constraints")
}

func (c *UnitCommandBase) Init(args []string) error {
	if c.NumUnits < 1 {
		return errors.New("--num-units must be a positive integer")
	}
	if c.ToMachineSpec != "" {
		if c.NumUnits > 1 {
			return errors.New("cannot use --num-units > 1 with --to")
		}
		if !cmd.IsMachineOrNewContainer(c.ToMachineSpec) {
			return fmt.Errorf("invalid --to parameter %q", c.ToMachineSpec)
		}
	}
	return nil
}

// AddUnitCommand is responsible adding additional units to a service.
type AddUnitCommand struct {
	cmd.EnvCommandBase
	UnitCommandBase
	ServiceName string
}

const addUnitDoc = `
Service units can be added to a specific machine using the --to argument.
Examples:
 juju add-unit mysql --to 23       (Add unit to machine 23)
 juju add-unit mysql --to 24/lxc/3 (Add unit to lxc container 3 on host machine 24)
 juju add-unit mysql --to lxc:25   (Add unit to a new lxc container on host machine 25)
`

func (c *AddUnitCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "add-unit",
		Args:    "<service name>",
		Purpose: "add a service unit",
		Doc:     addUnitDoc,
	}
}

func (c *AddUnitCommand) SetFlags(f *gnuflag.FlagSet) {
	c.EnvCommandBase.SetFlags(f)
	c.UnitCommandBase.SetFlags(f)
	f.IntVar(&c.NumUnits, "n", 1, "number of service units to add")
}

func (c *AddUnitCommand) Init(args []string) error {
	switch len(args) {
	case 1:
		c.ServiceName = args[0]
	case 0:
		return errors.New("no service specified")
	}
	if err := cmd.CheckEmpty(args[1:]); err != nil {
		return err
	}
	return c.UnitCommandBase.Init(args)
}

// Run connects to the environment specified on the command line
// and calls conn.AddUnits.
func (c *AddUnitCommand) Run(_ *cmd.Context) error {
	conn, err := juju.NewConnFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer conn.Close()

	params := params.AddServiceUnits{
		ServiceName:   c.ServiceName,
		NumUnits:      c.NumUnits,
		ToMachineSpec: c.ToMachineSpec,
	}
	_, err = statecmd.AddServiceUnits(conn.State, params)
	return err
}
