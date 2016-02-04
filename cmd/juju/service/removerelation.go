// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"

	apiservice "github.com/juju/juju/api/service"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
)

// NewRemoveRelationCommand returns a command to remove a relation between 2 services.
func NewRemoveRelationCommand() cmd.Command {
	return modelcmd.Wrap(&removeRelationCommand{})
}

// removeRelationCommand causes an existing service relation to be shut down.
type removeRelationCommand struct {
	modelcmd.ModelCommandBase
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

type serviceDestroyRelationAPI interface {
	Close() error
	DestroyRelation(endpoints ...string) error
}

func (c *removeRelationCommand) getAPI() (serviceDestroyRelationAPI, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return apiservice.NewClient(root), nil
}

func (c *removeRelationCommand) Run(_ *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer client.Close()
	return block.ProcessBlockedError(client.DestroyRelation(c.Endpoints...), block.BlockRemove)
}
