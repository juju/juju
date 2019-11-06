// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/api/modelgeneration"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
)

const (
	showCommitsSummary = "Shows the commit content"
	showCommitsDoc     = `
show-commit shows the effective change from a branch to the master.

Examples:
    juju show-commits 3

See also:
	list-commits
    add-branch
    track
    branch
    abort
    diff
`
)

//TODO: instead of diffing, i just show the content of config
//gen-id is unique corresponds to commit it to show

// NewCommitCommand wraps listCommitsCommand with sane model settings.
func NewShowCommitCommand() cmd.Command {
	return modelcmd.Wrap(&ShowCommitCommand{})
}

// ShowCommitCommand supplies the "show-commit" CLI command used to show commits
type ShowCommitCommand struct {
	modelcmd.ModelCommandBase

	api ShowCommitCommandAPI
}

// ShowCommitCommandAPI defines an API interface to be used during testing.
//go:generate mockgen -package mocks -destination ./mocks/commit_mock.go github.com/juju/juju/cmd/juju/model ShowCommitCommandAPI
type ShowCommitCommandAPI interface {
	Close() error

	// ListCommitsBranch commits the branch with the input name to the model,
	// effectively completing it and applying
	// all branch changes across the model.
	// The new generation ID of the model is returned.
	ShowCommit() (int, error)
}

// Info implements part of the cmd.Command interface.
func (c *ShowCommitCommand) Info() *cmd.Info {
	info := &cmd.Info{
		Name:    "list-commits",
		Purpose: listCommitsSummary,
		Doc:     listCommitsDoc,
	}
	return jujucmd.Info(info)
}

// SetFlags implements part of the cmd.Command interface.
func (c *ShowCommitCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
}

// Init implements part of the cmd.Command interface.
func (c *ShowCommitCommand) Init(args []string) error {
	return nil
}

// getAPI returns the API. This allows passing in a test CommitCommandAPI
// implementation.
func (c *ShowCommitCommand) getAPI() (ShowCommitCommandAPI, error) {
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
func (c *ShowCommitCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	_, err = client.ShowCommit()
	if err != nil {
		return err
	}
	return err
}
