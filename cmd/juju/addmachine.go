// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"strings"
)

// AddMachineCommand starts a new machine and registers it in the environment.
type AddMachineCommand struct {
	EnvCommandBase
	// If specified, use this series, else use the environment default-series
	Series string
	// If specified, these constraints are merged with those already in the environment.
	MachineConstraints constraints.Value
	// If specified, these constraints must be compatible with any machine constraints.
	ContainerConstraints constraints.Value
	MachineId            string
	ContainerType        state.ContainerType
}

func (c *AddMachineCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "add-machine",
		Args:    "[<machine>/<container> | /<container>]",
		Purpose: "start a new, empty machine and optionally a container, or add a container to a machine",
		Doc:     "Machines are created in a clean state and ready to have units deployed.",
	}
}

func (c *AddMachineCommand) SetFlags(f *gnuflag.FlagSet) {
	c.EnvCommandBase.SetFlags(f)
	f.StringVar(&c.Series, "series", "", "the charm series")
	f.Var(constraints.ConstraintsValue{&c.MachineConstraints}, "constraints", "additional machine constraints")
	f.Var(constraints.ConstraintsValue{&c.ContainerConstraints}, "container-constraints", "constraints for the container")
}

func (c *AddMachineCommand) Init(args []string) error {
	containerSpec, err := cmd.ZeroOrOneArgs(args)
	if err != nil {
		return err
	}
	if containerSpec == "" {
		if c.ContainerConstraints.String() != "" {
			return fmt.Errorf("container constraints not applicable when no container is specified")
		}
		return nil
	}
	// container arg can either be 'machine/type' or '/type'
	sep := strings.Index(containerSpec, "/")
	if sep < 0 {
		return fmt.Errorf("malformed container argument %q", containerSpec)
	}
	c.MachineId, c.ContainerType = containerSpec[:sep], state.ContainerType(containerSpec[sep+1:])
	if c.MachineId != "" && c.MachineConstraints.String() != "" {
		return fmt.Errorf("machine constraints not applicable when parent machine is specified")
	}
	for _, supportedType := range state.SupportedContainerTypes {
		if c.ContainerType == supportedType {
			return nil
		}
	}
	return fmt.Errorf("invalid container type %q", c.ContainerType)
}

func (c *AddMachineCommand) Run(_ *cmd.Context) error {
	conn, err := juju.NewConnFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer conn.Close()

	series := c.Series
	if series == "" {
		conf, err := conn.State.EnvironConfig()
		if err != nil {
			return err
		}
		series = conf.DefaultSeries()
	}
	if c.ContainerType == "" {
		m, err := conn.State.AddMachineWithConstraints(series, c.MachineConstraints, state.JobHostUnits)
		if err != nil {
			log.Infof("created machine %v", m)
		}
		return err
	} else {
		m, err := conn.State.AddContainerWithConstraints(
			c.MachineId, c.ContainerType, series, c.MachineConstraints, c.ContainerConstraints, state.JobHostUnits)
		if err == nil {
			log.Infof("created %q container on machine %v", c.ContainerType, m)
		}
		return err
	}
	return err
}
