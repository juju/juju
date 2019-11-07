// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/core/model"
	"time"

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

// NewCommitCommand wraps listCommitsCommand with sane model settings.
func NewShowCommitCommand() cmd.Command {
	return modelcmd.Wrap(&ShowCommitCommand{})
}

// ShowCommitCommand supplies the "show-commit" CLI command used to show commits
type ShowCommitCommand struct {
	modelcmd.ModelCommandBase

	api ShowCommitCommandAPI
	out cmd.Output

	isoTime bool
}

// ShowCommitCommandAPI defines an API interface to be used during testing.
//go:generate mockgen -package mocks -destination ./mocks/showcommit_mock.go github.com/juju/juju/cmd/juju/model ShowCommitCommandAPI
type ShowCommitCommandAPI interface {
	Close() error

	// ListCommitsBranch commits the branch with the input name to the model,
	// effectively completing it and applying
	// all branch changes across the model.
	// The new generation ID of the model is returned.
	ShowCommit(func(time.Time) string) (model.GenerationCommit, error)
}

// Info implements part of the cmd.Command interface.
func (c *ShowCommitCommand) Info() *cmd.Info {
	info := &cmd.Info{
		Name:    "show-commit",
		Purpose: showCommitsDoc,
		Doc:     showCommitsSummary,
	}
	return jujucmd.Info(info)
}

// SetFlags implements part of the cmd.Command interface.
func (c *ShowCommitCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.BoolVar(&c.isoTime, "utc", false, "Display time as UTC in RFC3339 format")
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
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

	formatTime := func(t time.Time) string {
		return common.FormatTime(&t, c.isoTime)
	}
	cmt, err := client.ShowCommit(formatTime)
	if err != nil {
		return err
	}
	return c.out.Write(ctx, cmt)
}
