// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"

	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/cmd/envcmd"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/juju"
)

type EnsureAvailabilityCommand struct {
	envcmd.EnvCommandBase
	NumStateServers int
	// If specified, use this series for newly created machines,
	// else use the environment's default-series
	Series string
	// If specified, these constraints will be merged with those
	// already in the environment when creating new machines.
	Constraints constraints.Value
}

const ensureAvailabilityDoc = `
To ensure availability of deployed services, the Juju infrastructure
must itself be highly available.  Ensure-availability must be called
to ensure that the specified number of state servers are made available.

An odd number of state servers is required.

Examples:
 juju ensure-availability -n 3
     Ensure that 3 state servers are available,
     with newly created state server machines
     having the default series and constraints.
 juju ensure-availability -n 5 --series=trusty
     Ensure that 5 state servers are available,
     with newly created state server machines
     having the "trusty" series.
 juju ensure-availability -n 7 --constraints mem=8G
     Ensure that 7 state servers are available,
     with newly created state server machines
     having the default series, and at least
     8GB RAM.
`

func (c *EnsureAvailabilityCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "ensure-availability",
		Purpose: "ensure the availability of Juju state servers",
		Doc:     ensureAvailabilityDoc,
	}
}

func (c *EnsureAvailabilityCommand) SetFlags(f *gnuflag.FlagSet) {
	c.EnvCommandBase.SetFlags(f)
	f.IntVar(&c.NumStateServers, "n", -1, "number of state servers to make available")
	f.StringVar(&c.Series, "series", "", "the charm series")
	f.Var(constraints.ConstraintsValue{&c.Constraints}, "constraints", "additional machine constraints")
}

func (c *EnsureAvailabilityCommand) Init(args []string) error {
	err := c.EnvCommandBase.Init()
	if err != nil {
		return err
	}
	if c.NumStateServers%2 != 1 || c.NumStateServers <= 0 {
		return fmt.Errorf("must specify a number of state servers odd and greater than zero")
	}
	return cmd.CheckEmpty(args)
}

// Run connects to the environment specified on the command line
// and calls EnsureAvailability.
func (c *EnsureAvailabilityCommand) Run(_ *cmd.Context) error {
	client, err := juju.NewAPIClientFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer client.Close()
	return client.EnsureAvailability(c.NumStateServers, c.Constraints, c.Series)
}
