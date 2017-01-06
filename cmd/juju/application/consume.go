// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/api/application"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/crossmodel"
)

var usageConsumeSummary = `
Add a remote application to the model.`[1:]

var usageConsumeDetails = `
Adds a remote application to the model. Relations can be created later using "juju relate".

The remote application can be identified in two ways:
    [<model owner>/]<model name>.<application name>
        for an application in another model in this controller (if owner isn't specified it's assumed to be the logged-in user)
or
    <remote endpoint url>
        for remote applications that have been shared using the offer command

Examples:
    $ juju consume othermodel.mysql

    $ juju consume local:/u/fred/db2

See also:
    add-relation
    offer`[1:]

// NewConsumeCommand returns a command to add remote applications to
// the model.
func NewConsumeCommand() cmd.Command {
	return modelcmd.Wrap(&consumeCommand{})
}

// consumeCommand adds remote applications to the model without
// relating them to other applications.
type consumeCommand struct {
	modelcmd.ModelCommandBase
	api               serviceConsumeAPI
	remoteApplication string
}

// Info implements cmd.Command.
func (c *consumeCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "consume",
		Args:    "<remote application>",
		Purpose: usageConsumeSummary,
		Doc:     usageConsumeDetails,
	}
}

// Init implements cmd.Command.
func (c *consumeCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.New("no remote application specified")
	}
	c.remoteApplication = args[0]
	url, err := crossmodel.ParseApplicationURL(c.remoteApplication)
	if err != nil {
		return errors.Trace(err)
	}
	if url.HasEndpoint() {
		return errors.New("remote application shouldn't include endpoint")
	}
	return cmd.CheckEmpty(args[1:])
}

func (c *consumeCommand) getAPI() (serviceConsumeAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return application.NewClient(root), nil
}

// Run adds the requested remote application to the model. Implements
// cmd.Command.
func (c *consumeCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer client.Close()
	localName, err := client.Consume(c.remoteApplication)
	if err != nil {
		return errors.Trace(err)
	}
	ctx.Infof("Added %s as %s", c.remoteApplication, localName)
	return nil
}

type serviceConsumeAPI interface {
	Close() error
	Consume(remoteApplication string) (string, error)
}
