// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"os"
	"strconv"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/api/modelgeneration"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/juju/osenv"
)

const (
	showCommitsSummary = "Displays details of the commit"
	showCommitsDoc     = `
Show-commit shows the committed branches to the model.
Details displayed include:
- user who committed the branch 
- when the branch was committed 
- user who created the branch 
- when the branch was created 
- configuration  made under the branch for each application
- a summary of how many units are tracking the branch

Examples:
    juju show-commits 3
    juju show-commits 3 --utc

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

// ShowCommitCommand supplies the "show-commit" CLI command used to show commits
type ShowCommitCommand struct {
	modelcmd.ModelCommandBase

	api ShowCommitCommandAPI
	out cmd.Output

	generationId int
	isoTime      bool
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

// NewCommitCommand wraps listCommitsCommand with sane model settings.
func NewShowCommitCommand() cmd.Command {
	return modelcmd.Wrap(&ShowCommitCommand{})
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
	c.out.AddFlags(f, "yaml", output.DefaultFormatters)
}

// Init implements part of the cmd.Command interface.
func (c *ShowCommitCommand) Init(args []string) error {
	lArgs := len(args)
	if lArgs != 1 {
		return errors.Errorf("expected exactly 1 commit id, got %d arguments", lArgs)
	}
	id, err := strconv.Atoi(args[0])
	if err != nil {
		return errors.Errorf("encountered problem trying to parse %q into an int", args[0])
	}
	c.generationId = id

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
