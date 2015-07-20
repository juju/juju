// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"

	"github.com/juju/cmd"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/block"
)

// AddRelationCommand adds a relation between two service endpoints.
type AddRelationCommand struct {
	envcmd.EnvCommandBase
	Endpoints []string
}

func (c *AddRelationCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "add-relation",
		Args:    "<service1>[:<relation name1>] <service2>[:<relation name2>]",
		Purpose: "add a relation between two services",
	}
}

func (c *AddRelationCommand) Init(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("a relation must involve two services")
	}
	c.Endpoints = args
	return nil
}

func (c *AddRelationCommand) Run(_ *cmd.Context) error {
	client, err := c.NewAPIClient()
	if err != nil {
		return err
	}
	defer client.Close()
	_, err = client.AddRelation(c.Endpoints...)
	return block.ProcessBlockedError(err, block.BlockChange)
}
