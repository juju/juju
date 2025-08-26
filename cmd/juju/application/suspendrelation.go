// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"strconv"
	"strings"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/api/client/application"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
)

var suspendHelpSummary = `
Suspends a relation to an application offer.`[1:]

var suspendHelpDetails = `
A relation between an application in another model and an offer in this model will be suspended.
The ` + "`relation-departed`" + ` and ` + "`relation-broken`" + ` hooks will be run for the relation, and the relation
status will be set to suspended. The relation is specified using its ID.
`

const suspendHelpExamples = `
    juju suspend-relation 123
    juju suspend-relation 123 --message "reason for suspending"
    juju suspend-relation 123 456 --message "reason for suspending"
`

// NewSuspendRelationCommand returns a command to suspend a relation.
func NewSuspendRelationCommand() cmd.Command {
	cmd := &suspendRelationCommand{}
	cmd.newAPIFunc = func() (SetRelationSuspendedAPI, error) {
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
	relationIds []int
	message     string
	newAPIFunc  func() (SetRelationSuspendedAPI, error)
}

func (c *suspendRelationCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "suspend-relation",
		Args:     "<relation-id>[ <relation-id>...]",
		Purpose:  suspendHelpSummary,
		Doc:      suspendHelpDetails,
		Examples: suspendHelpExamples,
		SeeAlso: []string{
			"integrate",
			"offers",
			"remove-relation",
			"resume-relation",
		},
	})
}

func (c *suspendRelationCommand) Init(args []string) (err error) {
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

func (c *suspendRelationCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.StringVar(&c.message, "message", "", "Reason for suspension")
}

// SetRelationSuspendedAPI defines the API methods that the suspend/resume relation commands use.
type SetRelationSuspendedAPI interface {
	Close() error
	SetRelationSuspended(relationIds []int, suspended bool, message string) error
}

func (c *suspendRelationCommand) Run(_ *cmd.Context) error {
	client, err := c.newAPIFunc()
	if err != nil {
		return err
	}
	defer client.Close()
	err = client.SetRelationSuspended(c.relationIds, true, c.message)
	return block.ProcessBlockedError(err, block.BlockChange)
}
