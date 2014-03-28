// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"

	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/juju"
)

type EnsureHACommand struct {
	cmd.EnvCommandBase
	NumStateServers int
	// If specified, use this series, else use the environment default-series
	Series string
	// If specified, these constraints are merged with those already in the environment.
	Constraints constraints.Value
}

const ensureHADoc = `
To ensure availability of deployed services, the Juju infrastructure
must itself be highly available.  Ensure-ha must be called to ensure
that the specified number of state servers are made available.

An odd number of state servers is required.

Examples:
 juju ensure-ha -n 3
     Ensure that 3 state servers are available,
     with newly created state server machines
     having the default series and constraints.
 juju ensure-ha -n 5 --series=trusty
     Ensure that 5 state servers are available,
     with newly created state server machines
     having the "trusty" series.
 juju ensure-ha -n 7 --constraints mem=8G
     Ensure that 7 state servers are available,
     with newly created state server machines
     having the default series, and at least
     8GB RAM.
`

func (c *EnsureHACommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "ensure-ha",
		Purpose: "ensure the availability of Juju state servers",
		Doc:     ensureHADoc,
	}
}

func (c *EnsureHACommand) SetFlags(f *gnuflag.FlagSet) {
	c.EnvCommandBase.SetFlags(f)
	f.IntVar(&c.NumStateServers, "n", 1, "number of state servers to make available")
	f.StringVar(&c.Series, "series", "", "the charm series")
	f.Var(constraints.ConstraintsValue{&c.Constraints}, "constraints", "additional machine constraints")
}

func (c *EnsureHACommand) Init(args []string) error {
	if c.NumStateServers%2 != 1 || c.NumStateServers <= 0 {
		return fmt.Errorf("number of state servers must be odd and greater than zero")
	}
	return cmd.CheckEmpty(args)
}

// Run connects to the environment specified on the command line
// and calls EnsureAvailability.
func (c *EnsureHACommand) Run(_ *cmd.Context) error {
	client, err := juju.NewAPIClientFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer client.Close()
	return client.EnsureAvailability(c.NumStateServers, c.Constraints, c.Series)
}
