// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"errors"
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/statecmd"
)

// UnitCommandBase provides support for commands which deploy units. It handles the parsing
// and validation of force-machine and num-units arguments.
type UnitCommandBase struct {
	ForceMachineSpec string
	NumUnits         int
}

func (c *UnitCommandBase) SetFlags(f *gnuflag.FlagSet) {
	f.IntVar(&c.NumUnits, "num-units", 1, "")
	f.StringVar(&c.ForceMachineSpec, "force-machine", "", "Machine/container to deploy unit, bypasses constraints")
}

func (c *UnitCommandBase) Init(args []string) error {
	if c.NumUnits < 1 {
		return errors.New("--num-units must be a positive integer")
	}
	if c.ForceMachineSpec != "" {
		if c.NumUnits > 1 {
			return errors.New("cannot use --num-units with --force-machine")
		}
		if !state.IsMachineOrNewContainer(c.ForceMachineSpec) {
			return fmt.Errorf("invalid force machine parameter %q", c.ForceMachineSpec)
		}
	}
	return nil
}

// AddUnitCommand is responsible adding additional units to a service.
type AddUnitCommand struct {
	EnvCommandBase
	UnitCommandBase
	ServiceName string
}

func (c *AddUnitCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "add-unit",
		Purpose: "add a service unit",
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
	default:
		return cmd.CheckEmpty(args[1:])
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
		ServiceName:      c.ServiceName,
		NumUnits:         c.NumUnits,
		ForceMachineSpec: c.ForceMachineSpec,
	}
	_, err = statecmd.AddServiceUnits(conn.State, params)
	return err
}
