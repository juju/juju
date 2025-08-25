// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"strconv"
	"strings"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"

	"github.com/juju/juju/api/client/application"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
)

var resumeHelpSummary = `
Resumes a suspended relation to an application offer.`[1:]

var resumeHelpDetails = `
A relation between an application in another model and an offer in this model will be resumed.
The ` + "`relation-joined`" + ` and ` + "`relation-changed`" + ` hooks will be run for the relation, and the relation
status will be set to joined. The relation is specified using its ID.
`

const resumeHelpExamples = `
    juju resume-relation 123
    juju resume-relation 123 456
`

// NewResumeRelationCommand returns a command to resume a relation.
func NewResumeRelationCommand() cmd.Command {
	cmd := &resumeRelationCommand{}
	cmd.newAPIFunc = func() (SetRelationSuspendedAPI, error) {
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
	relationIds []int
	newAPIFunc  func() (SetRelationSuspendedAPI, error)
}

func (c *resumeRelationCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "resume-relation",
		Args:     "<relation-id>[,<relation-id>]",
		Purpose:  resumeHelpSummary,
		Doc:      resumeHelpDetails,
		Examples: resumeHelpExamples,
		SeeAlso: []string{
			"integrate",
			"offers",
			"remove-relation",
			"suspend-relation",
		},
	})
}

func (c *resumeRelationCommand) Init(args []string) (err error) {
	if len(args) == 0 {
		return errors.New("no relation ids specified")
	}
	for _, id := range args {
		if relId, err := strconv.Atoi(strings.TrimSpace(id)); err != nil || relId < 0 {
			return errors.NotValidf("relation ID %q", id)
		} else {
			c.relationIds = append(c.relationIds, relId)
		}
	}
	return nil
}

func (c *resumeRelationCommand) Run(_ *cmd.Context) error {
	client, err := c.newAPIFunc()
	if err != nil {
		return err
	}
	defer client.Close()
	err = client.SetRelationSuspended(c.relationIds, false, "")
	return block.ProcessBlockedError(err, block.BlockChange)
}
