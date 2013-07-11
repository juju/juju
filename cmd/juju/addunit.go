// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"errors"
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/statecmd"
	"strings"
)

// UnitCommandBase provides support for commands which deploy units. It handles the parsing
// and validation of force-machine and num-units arguments.
type UnitCommandBase struct {
	ForceMachineSpec   string
	ForceMachineId     string
	ForceContainerType instance.ContainerType
	NumUnits           int
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
		// Force machine spec may be an existing machine or container, eg 3/lxc/2
		// or a new container on a machine, eg 3/lxc
		specParts := strings.Split(c.ForceMachineSpec, "/")
		if len(specParts) == 1 {
			c.ForceMachineId = specParts[0]
		} else {
			lastPart := specParts[len(specParts)-1]
			var err error
			if c.ForceContainerType, err = instance.ParseSupportedContainerType(lastPart); err == nil {
				c.ForceMachineId = strings.Join(specParts[:len(specParts)-1], "/")
			} else {
				c.ForceMachineId = c.ForceMachineSpec
			}
		}
		if !state.IsMachineId(c.ForceMachineId) {
			return fmt.Errorf("invalid force machine id %q", c.ForceMachineId)
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
		ServiceName:        c.ServiceName,
		NumUnits:           c.NumUnits,
		ForceMachineId:     c.ForceMachineId,
		ForceContainerType: c.ForceContainerType,
	}
	_, err = statecmd.AddServiceUnits(conn.State, params)
	return err
}
