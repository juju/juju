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

var resumeHelpSummary = `
Resumes a suspended relation to an application offer.`[1:]

var resumeHelpDetails = `
A relation between an application in another model and an offer in this model will be resumed. 
The relation-joined and relation-changed hooks will be run for the relation, and the relation
status will be set to joined. The relation is specified using its id.

Examples:
    juju resume-relation 123

See also: 
    add-relation
    offers
    remove-relation
    suspend-relation`

// NewResumeRelationCommand returns a command to resume a relation.
func NewResumeRelationCommand() cmd.Command {
	cmd := &resumeRelationCommand{}
	cmd.newAPIFunc = func() (SetRelationStatusAPI, error) {
		root, err := cmd.NewAPIRoot()
		if err != nil {
			return nil, errors.Trace(err)
		}
		return application.NewClient(root), nil

	}
	return modelcmd.Wrap(cmd)
}

type resumeRelationCommand struct {
	modelcmd.ModelCommandBase
	RelationId int
	newAPIFunc func() (SetRelationStatusAPI, error)
}

func (c *resumeRelationCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "resume-relation",
		Args:    "<relation-id>",
		Purpose: resumeHelpSummary,
		Doc:     resumeHelpDetails,
	}
}

func (c *resumeRelationCommand) Init(args []string) (err error) {
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

func (c *resumeRelationCommand) Run(_ *cmd.Context) error {
	client, err := c.newAPIFunc()
	if err != nil {
		return err
	}
	defer client.Close()
	if client.BestAPIVersion() < 5 {
		return errors.New("resuming a relation is not supported by this version of Juju")
	}
	err = client.SetRelationStatus(c.RelationId, relation.Joined)
	return block.ProcessBlockedError(err, block.BlockChange)
}
