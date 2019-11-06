// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/juju/osenv"
	"os"
	"strconv"
	"time"

	"github.com/juju/juju/api/modelgeneration"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
)

const (
	listCommitsSummary = "Lists commits history"
	listCommitsDoc     = `
commits shows the timeline of changes to the model that occurred through branching.
It does not take into account other changes to the model that did not occur through a managed branch.

Examples:
    juju commits

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

// CommitsCommand supplies the "commit" CLI command used to commit changes made
// under a branch, to the model.
type CommitsCommand struct {
	modelcmd.ModelCommandBase

	api CommitsCommandAPI
	out cmd.Output

	isoTime bool
}

// CommitsCommandAPI defines an API interface to be used during testing.
//go:generate mockgen -package mocks -destination ./mocks/commits_mock.go github.com/juju/juju/cmd/juju/model CommitsCommandAPI
type CommitsCommandAPI interface {
	Close() error

	// ListCommitsBranch commits the branch with the input name to the model,
	// effectively completing it and applying
	// all branch changes across the model.
	// The new generation ID of the model is returned.
	ListCommits(func(time.Time) string) (model.GenerationCommits, error)
}

// NewCommitCommand wraps CommitsCommand with sane model settings.
func NewCommitsCommand() cmd.Command {
	return modelcmd.Wrap(&CommitsCommand{})
}

// Info implements part of the cmd.Command interface.
func (c *CommitsCommand) Info() *cmd.Info {
	info := &cmd.Info{
		Name:    "commits",
		Purpose: listCommitsSummary,
		Doc:     listCommitsDoc,
		Aliases: []string{"list-commits"},
	}
	return jujucmd.Info(info)
}

// SetFlags implements part of the cmd.Command interface.
func (c *CommitsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.BoolVar(&c.isoTime, "utc", false, "Display time as UTC in RFC3339 format")
}

// Init implements part of the cmd.Command interface.
func (c *CommitsCommand) Init(args []string) error {
	lArgs := len(args)
	if lArgs > 0 {
		return errors.Errorf("expected no arguments, but got %v", lArgs)
	}

	// If use of ISO time not specified on command line, check env var.
	if !c.isoTime {
		var err error
		envVarValue := os.Getenv(osenv.JujuStatusIsoTimeEnvKey)
		if envVarValue != "" {
			if c.isoTime, err = strconv.ParseBool(envVarValue); err != nil {
				return errors.Annotatef(err, "invalid %s env var, expected true|false", osenv.JujuStatusIsoTimeEnvKey)
			}
		}
	}
	return nil
}

// getAPI returns the API. This allows passing in a test CommitCommandAPI
// implementation.
func (c *CommitsCommand) getAPI() (CommitsCommandAPI, error) {
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
func (c *CommitsCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()
	// Partially apply our time format
	formatTime := func(t time.Time) string {
		return common.FormatTime(&t, c.isoTime)
	}

	commits, err := client.ListCommits(formatTime)
	if err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(c.out.Write(ctx, commits))
}
