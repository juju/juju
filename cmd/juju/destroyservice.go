// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/statecmd"
)

// DestroyServiceCommand causes an existing service to be destroyed.
type DestroyServiceCommand struct {
	EnvCommandBase
	ServiceName string
}

func (c *DestroyServiceCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "destroy-service",
		Args:    "<service>",
		Purpose: "destroy a service",
		Doc:     "Destroying a service will destroy all its units and relations.",
	}
}

func (c *DestroyServiceCommand) Init(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no service specified")
	}
	if !names.IsService(args[0]) {
		return fmt.Errorf("invalid service name %q", args[0])
	}
	c.ServiceName, args = args[0], args[1:]
	return cmd.CheckEmpty(args)
}

func (c *DestroyServiceCommand) Run(ctx *cmd.Context) error {
	conn, err := juju.NewConnFromName(c.EnvName)
	if err != nil {
		return c.envOpenFailure(err, ctx.Stderr)
	}
	defer conn.Close()

	params := params.ServiceDestroy{
		ServiceName: c.ServiceName,
	}
	return statecmd.ServiceDestroy(conn.State, params)
}
