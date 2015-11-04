// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"

	"github.com/juju/cmd"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/block"
)

func newRemoveRelationCommand() cmd.Command {
	return envcmd.Wrap(&removeRelationCommand{})
}

// removeRelationCommand causes an existing service relation to be shut down.
type removeRelationCommand struct {
	envcmd.EnvCommandBase
	Endpoints []string
}

func (c *removeRelationCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "remove-relation",
		Args:    "<service1>[:<relation name1>] <service2>[:<relation name2>]",
		Purpose: "remove a relation between two services",
		Aliases: []string{"destroy-relation"},
	}
}

func (c *removeRelationCommand) Init(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("a relation must involve two services")
	}
	c.Endpoints = args
	return nil
}

func (c *removeRelationCommand) Run(_ *cmd.Context) error {
	client, err := c.NewAPIClient()
	if err != nil {
		return err
	}
	defer client.Close()
	return block.ProcessBlockedError(client.DestroyRelation(c.Endpoints...), block.BlockRemove)
}
