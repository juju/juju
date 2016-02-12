// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/api/service"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
)

// NewUnexposeCommand returns a command to unexpose services.
func NewUnexposeCommand() cmd.Command {
	return modelcmd.Wrap(&unexposeCommand{})
}

// unexposeCommand is responsible exposing services.
type unexposeCommand struct {
	modelcmd.ModelCommandBase
	ServiceName string
}

func (c *unexposeCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "unexpose",
		Args:    "<service>",
		Purpose: "unexpose a service",
	}
}

func (c *unexposeCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.New("no service name specified")
	}
	c.ServiceName = args[0]
	return cmd.CheckEmpty(args[1:])
}

func (c *unexposeCommand) getAPI() (serviceExposeAPI, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return service.NewClient(root), nil
}

// Run changes the juju-managed firewall to hide any
// ports that were also explicitly marked by units as closed.
func (c *unexposeCommand) Run(_ *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer client.Close()
	return block.ProcessBlockedError(client.Unexpose(c.ServiceName), block.BlockChange)
}
