// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
)

// CreateMachineCommand starts a new machine and registers it in the environment.
type CreateMachineCommand struct {
	EnvCommandBase
	// If specified, these constraints are merged with those already in the environment.
	Constraints constraints.Value
}

func (c *CreateMachineCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "create-machine",
		Purpose: "create machines",
		Doc:     "Machines are created in a clean state and ready to have units deployed.",
	}
}

func (c *CreateMachineCommand) SetFlags(f *gnuflag.FlagSet) {
	c.EnvCommandBase.SetFlags(f)
	f.Var(constraints.ConstraintsValue{&c.Constraints}, "constraints", "additional machine constraints")
}

func (c *CreateMachineCommand) Init(args []string) error {
	return cmd.CheckEmpty(args)
}

func (c *CreateMachineCommand) Run(_ *cmd.Context) error {
	conn, err := juju.NewConnFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer conn.Close()

	conf, err := conn.State.EnvironConfig()
	if err != nil {
		return err
	}

	m, err := conn.State.AddMachineWithConstraints(conf.DefaultSeries(), &c.Constraints, state.JobHostUnits)
	log.Infof("created machine %v", m)
	return err
}
