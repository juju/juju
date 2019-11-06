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
	listCommitsSummary = "Lists commits history"
	listCommitsDoc     = `
List commits shows the timeline of changes to the model that occurred through branching.
It does not take into account other changes to the model that did not occur through a managed branch.

Examples:
    juju list-commits

See also:
	commits
	show-commit
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
func NewListCommitCommand() cmd.Command {
	return modelcmd.Wrap(&listCommitsCommand{})
}

// listCommitsCommand supplies the "commit" CLI command used to commit changes made
// under a branch, to the model.
type listCommitsCommand struct {
	modelcmd.ModelCommandBase

	api ListCommitsCommandAPI
}

// ListCommitsCommandAPI defines an API interface to be used during testing.
//go:generate mockgen -package mocks -destination ./mocks/commit_mock.go github.com/juju/juju/cmd/juju/model ListCommitsCommandAPI
type ListCommitsCommandAPI interface {
	Close() error

	// ListCommitsBranch commits the branch with the input name to the model,
	// effectively completing it and applying
	// all branch changes across the model.
	// The new generation ID of the model is returned.
	ListCommits() (int, error)
}

// Info implements part of the cmd.Command interface.
func (c *listCommitsCommand) Info() *cmd.Info {
	info := &cmd.Info{
		Name:    "list-commits",
		Purpose: listCommitsSummary,
		Doc:     listCommitsDoc,
	}
	return jujucmd.Info(info)
}

// SetFlags implements part of the cmd.Command interface.
func (c *listCommitsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
}

// Init implements part of the cmd.Command interface.
func (c *listCommitsCommand) Init(args []string) error {
	return nil
}

// getAPI returns the API. This allows passing in a test CommitCommandAPI
// implementation.
func (c *listCommitsCommand) getAPI() (ListCommitsCommandAPI, error) {
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
func (c *listCommitsCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	_, err = client.ListCommits()
	if err != nil {
		return err
	}
	return err
}
