// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"strconv"

	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/api/application"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/relation"
)

var suspendHelpSummary = `
Suspends a relation to an application offer.`[1:]

var suspendHelpDetails = `
A relation between an application in another model and an offer in this model will be suspended. 
The relation-departed and relation-broken hooks will be run for the relation, and the relation
status will be set to suspended. The relation is specified using its id.

Examples:
    juju suspend-relation 123

See also: 
    add-relation
    offers
    remove-relation
    resume-relation`

// NewSuspendRelationCommand returns a command to suspend a relation.
func NewSuspendRelationCommand() cmd.Command {
	cmd := &suspendRelationCommand{}
	cmd.newAPIFunc = func() (SetRelationStatusAPI, error) {
		root, err := cmd.NewAPIRoot()
		if err != nil {
			return nil, errors.Trace(err)
		}
		return application.NewClient(root), nil

	}
	return modelcmd.Wrap(cmd)
}

type suspendRelationCommand struct {
	modelcmd.ModelCommandBase
	RelationId int
	newAPIFunc func() (SetRelationStatusAPI, error)
}

func (c *suspendRelationCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "suspend-relation",
		Args:    "<relation-id>",
		Purpose: suspendHelpSummary,
		Doc:     suspendHelpDetails,
	}
}

func (c *suspendRelationCommand) Init(args []string) (err error) {
	if len(args) == 1 {
		if c.RelationId, err = strconv.Atoi(args[0]); err != nil || c.RelationId < 0 {
			return errors.NotValidf("relation ID %q", args[0])
		}
		return nil
	}
	if len(args) == 0 {
		return errors.New("no relation id specified")
	}
	return cmd.CheckEmpty(args[1:])
}

// SetRelationStatusAPI defines the API methods that the suspend/resume relation commands use.
type SetRelationStatusAPI interface {
	Close() error
	BestAPIVersion() int
	SetRelationStatus(relationId int, status relation.Status) error
}

func (c *suspendRelationCommand) Run(_ *cmd.Context) error {
	client, err := c.newAPIFunc()
	if err != nil {
		return err
	}
	defer client.Close()
	if client.BestAPIVersion() < 5 {
		return errors.New("suspending a relation is not supported by this version of Juju")
	}
	err = client.SetRelationStatus(c.RelationId, relation.Suspended)
	return block.ProcessBlockedError(err, block.BlockChange)
}
