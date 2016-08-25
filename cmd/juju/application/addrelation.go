// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/api/application"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
)

// NewAddRelationCommand returns a command to add a relation between 2 services.
func NewAddRelationCommand() cmd.Command {
	return modelcmd.Wrap(&addRelationCommand{})
}

// addRelationCommand adds a relation between two application endpoints.
type addRelationCommand struct {
	modelcmd.ModelCommandBase
	Endpoints []string
}

func (c *addRelationCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "add-relation",
		Aliases: []string{"relate"},
		Args:    "<application1>[:<relation name1>] <application2>[:<relation name2>]",
		Purpose: "Add a relation between two applications.",
	}
}

func (c *addRelationCommand) Init(args []string) error {
	if len(args) != 2 {
		return errors.Errorf("a relation must involve two applications")
	}
	c.Endpoints = args
	return nil
}

type serviceAddRelationAPI interface {
	Close() error
	AddRelation(endpoints ...string) (*params.AddRelationResults, error)
}

func (c *addRelationCommand) getAPI() (serviceAddRelationAPI, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return application.NewClient(root), nil
}

func (c *addRelationCommand) Run(_ *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer client.Close()
	_, err = client.AddRelation(c.Endpoints...)
	return block.ProcessBlockedError(err, block.BlockChange)
}
