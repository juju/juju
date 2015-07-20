// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"errors"

	"github.com/juju/cmd"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/block"
)

// UnexposeCommand is responsible exposing services.
type UnexposeCommand struct {
	envcmd.EnvCommandBase
	ServiceName string
}

func (c *UnexposeCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "unexpose",
		Args:    "<service>",
		Purpose: "unexpose a service",
	}
}

func (c *UnexposeCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.New("no service name specified")
	}
	c.ServiceName = args[0]
	return cmd.CheckEmpty(args[1:])
}

// Run changes the juju-managed firewall to hide any
// ports that were also explicitly marked by units as closed.
func (c *UnexposeCommand) Run(_ *cmd.Context) error {
	client, err := c.NewAPIClient()
	if err != nil {
		return err
	}
	defer client.Close()
	return block.ProcessBlockedError(client.ServiceUnexpose(c.ServiceName), block.BlockChange)
}
