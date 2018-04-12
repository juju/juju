// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"strconv"

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

The relation is specified using the relation endpoint names, eg
  mysql wordpress, or
  mediawiki:db mariadb:db

It is also possible to specify the relation ID, if known. This is useful to
terminate a relation originating from a different model, where only the ID is known. 

Examples:
    juju remove-relation mysql wordpress
    juju remove-relation 4

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
	cmd := &removeRelationCommand{}
	cmd.newAPIFunc = func() (ApplicationDestroyRelationAPI, error) {
		root, err := cmd.NewAPIRoot()
		if err != nil {
			return nil, errors.Trace(err)
		}
		return application.NewClient(root), nil

	}
	return modelcmd.Wrap(cmd)
}

// removeRelationCommand causes an existing application relation to be shut down.
type removeRelationCommand struct {
	modelcmd.ModelCommandBase
	RelationId int
	Endpoints  []string
	newAPIFunc func() (ApplicationDestroyRelationAPI, error)
}

func (c *removeRelationCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "remove-relation",
		Args:    "<application1>[:<relation name1>] <application2>[:<relation name2>] | <relation-id>",
		Purpose: helpSummary,
		Doc:     helpDetails,
	}
}

func (c *removeRelationCommand) Init(args []string) (err error) {
	if len(args) == 1 {
		if c.RelationId, err = strconv.Atoi(args[0]); err != nil || c.RelationId < 0 {
			return errors.NotValidf("relation ID %q", args[0])
		}
		return nil
	}
	if len(args) != 2 {
		return errors.Errorf("a relation must involve two applications")
	}
	c.Endpoints = args
	return nil
}

// ApplicationDestroyRelationAPI defines the API methods that application remove relation command uses.
type ApplicationDestroyRelationAPI interface {
	Close() error
	BestAPIVersion() int
	DestroyRelation(endpoints ...string) error
	DestroyRelationId(relationId int) error
}

func (c *removeRelationCommand) Run(_ *cmd.Context) error {
	client, err := c.newAPIFunc()
	if err != nil {
		return err
	}
	defer client.Close()
	if len(c.Endpoints) == 0 && client.BestAPIVersion() < 5 {
		return errors.New("removing a relation using its ID is not supported by this version of Juju")
	}
	if len(c.Endpoints) > 0 {
		err = client.DestroyRelation(c.Endpoints...)
	} else {
		err = client.DestroyRelationId(c.RelationId)
	}
	return block.ProcessBlockedError(err, block.BlockRemove)
}
