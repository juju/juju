// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/api/modelgeneration"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/model"
)

const (
	commitSummary = "Commits a branch to the model."
	commitDoc     = `
Committing a branch writes changes to charm configuration made under the 
branch, to the model. All units who's applications were changed under the 
branch realise those changes, as will any new units.

Examples:
    juju commit upgrade-postgresql

See also:
    branch
    track
    checkout
    abort
    diff
`
)

// NewCommitCommand wraps commitCommand with sane model settings.
func NewCommitCommand() cmd.Command {
	return modelcmd.Wrap(&commitCommand{})
}

// commitCommand supplies the "commit" CLI command used to commit changes made
// under a branch, to the model.
type commitCommand struct {
	modelcmd.ModelCommandBase

	api CommitCommandAPI

	branchName string
}

// CommitCommandAPI defines an API interface to be used during testing.
//go:generate mockgen -package mocks -destination ./mocks/commit_mock.go github.com/juju/juju/cmd/juju/model CommitCommandAPI
type CommitCommandAPI interface {
	Close() error
	CommitBranch(string, string) (int, error)
}

// Info implements part of the cmd.Command interface.
func (c *commitCommand) Info() *cmd.Info {
	info := &cmd.Info{
		Name:    "commit",
		Args:    "<branch name>",
		Purpose: commitSummary,
		Doc:     commitDoc,
	}
	return jujucmd.Info(info)
}

// SetFlags implements part of the cmd.Command interface.
func (c *commitCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
}

// Init implements part of the cmd.Command interface.
func (c *commitCommand) Init(args []string) error {
	if len(args) != 1 {
		return errors.Errorf("must specify a branch name to commit")
	}
	c.branchName = args[0]
	return nil
}

// getAPI returns the API. This allows passing in a test CommitCommandAPI
// implementation.
func (c *commitCommand) getAPI() (CommitCommandAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	api, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Annotate(err, "opening API connection")
	}
	client := modelgeneration.NewClient(api)
	return client, nil
}

// Run implements the meaty part of the cmd.Command interface.
func (c *commitCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	_, modelDetails, err := c.ModelCommandBase.ModelDetails()
	if err != nil {
		return errors.Annotate(err, "getting model details")
	}

	newGenId, err := client.CommitBranch(modelDetails.ModelUUID, c.branchName)
	if err != nil {
		return err
	}

	// Set the active branch to be the master.
	if err = c.SetActiveBranch(model.GenerationMaster); err != nil {
		return err
	}

	msg := fmt.Sprintf("Branch %q ", c.branchName)

	// If the model generation was not advanced, it means that there were no
	// changes to apply from the branch - it was aborted.
	if newGenId == 0 {
		msg = msg + "had no changes to commit and was aborted"
	} else {
		msg = msg + fmt.Sprintf("committed; model is now at generation %d", newGenId)
	}
	msg = msg + fmt.Sprintf("\nActive branch set to %q\n", model.GenerationMaster)

	_, err = ctx.Stdout.Write([]byte(msg))
	return err
}
