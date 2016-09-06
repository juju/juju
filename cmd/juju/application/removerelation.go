// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/api/application"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
)

var helpSummary = `
Removes an existing relation between two applications.`[1:]

var helpDetails = `
An existing relation between the two specified applications will be removed. 
This should not result in either of the applications entering an error state,
but may result in either or both of the applications being unable to continue
normal operation. In the case that there is more than one relation between
two applications it is necessary to specify which is to be removed (see
examples). Relations will automatically be removed when using the`[1:] + "`juju\nremove-application`" + ` command.

Examples:
    juju remove-relation mysql wordpress

In the case of multiple relations, the relation name should be specified
at least once - the following examples will all have the same effect:

    juju remove-relation mediawiki:db mariadb:db
    juju remove-relation mediawiki mariadb:db
    juju remove-relation mediawiki:db mariadb
 
See also: 
    add-relation
    remove-application`

// NewRemoveRelationCommand returns a command to remove a relation between 2 services.
func NewRemoveRelationCommand() cmd.Command {
	return modelcmd.Wrap(&removeRelationCommand{})
}

// removeRelationCommand causes an existing application relation to be shut down.
type removeRelationCommand struct {
	modelcmd.ModelCommandBase
	Endpoints []string
}

func (c *removeRelationCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "remove-relation",
		Args:    "<application1>[:<relation name1>] <application2>[:<relation name2>]",
		Purpose: helpSummary,
		Doc:     helpDetails,
	}
}

func (c *removeRelationCommand) Init(args []string) error {
	if len(args) != 2 {
		return errors.Errorf("a relation must involve two applications")
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
	return application.NewClient(root), nil
}

func (c *removeRelationCommand) Run(_ *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer client.Close()
	return block.ProcessBlockedError(client.DestroyRelation(c.Endpoints...), block.BlockRemove)
}
