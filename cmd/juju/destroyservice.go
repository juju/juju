// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/names"
)

// DestroyServiceCommand causes an existing service to be destroyed.
type DestroyServiceCommand struct {
	cmd.EnvCommandBase
	ServiceName string
}

func (c *DestroyServiceCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "destroy-service",
		Args:    "<service>",
		Purpose: "destroy a service",
		Doc:     "Destroying a service will destroy all its units and relations.",
		Aliases: []string{"remove-service"},
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

func (c *DestroyServiceCommand) Run(_ *cmd.Context) error {
	client, err := juju.NewAPIClientFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer client.Close()
	return client.ServiceDestroy(c.ServiceName)
}
