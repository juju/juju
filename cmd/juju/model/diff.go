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
	diffSummary = `Displays details of active branches.`
	diffDoc     = `
Details displayed include:
- user who created the branch
- when it was created
- configuration changes made under the branch for each application
- a summary of how many units are tracking the branch

Supplying the --all flag will show units tracking the branch and those still
tracking "master".

Examples:
    juju diff
	juju diff test-branch --all 	
    juju diff --utc

See also:
    add-branch
    track
    branch
    commit
    abort
`
)

// DiffCommandAPI describes API methods required to execute the diff command.
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination ./mocks/diff_mock.go github.com/juju/juju/cmd/juju/model DiffCommandAPI
type DiffCommandAPI interface {
	Close() error

	// BranchInfo returns information about "in-flight" branches.
	// If a non-empty string is supplied for branch name,
	// then only information for that branch is returned.
	// Supplying true for detailed returns extra unit detail for the branch.
	BranchInfo(branchName string, detailed bool, formatTime func(time.Time) string) (model.GenerationSummaries, error)
}

// diffCommand supplies the "diff" CLI command used to show information about
// active model branches.
type diffCommand struct {
	modelcmd.ModelCommandBase

	api DiffCommandAPI
	out cmd.Output

	isoTime    bool
	branchName string
	detailed   bool
}

// NewDiffCommand wraps diffCommand with sane model settings.
func NewDiffCommand() cmd.Command {
	return modelcmd.Wrap(&diffCommand{})
}

// Info implements part of the cmd.Command interface.
func (c *diffCommand) Info() *cmd.Info {
	info := &cmd.Info{
		Name:    "diff",
		Args:    "<branch name>",
		Purpose: diffSummary,
		Doc:     diffDoc,
	}
	return jujucmd.Info(info)
}

// SetFlags implements part of the cmd.Command interface.
func (c *diffCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.BoolVar(&c.isoTime, "utc", false, "Display time as UTC in RFC3339 format")
	f.BoolVar(&c.detailed, "all", false, "Show branch unit detail")
	c.out.AddFlags(f, "yaml", output.DefaultFormatters)
}

// Init implements part of the cmd.Command interface.
func (c *diffCommand) Init(args []string) error {
	lArgs := len(args)
	if lArgs > 1 {
		return errors.Errorf("expected at most 1 branch name, got %d arguments", lArgs)
	}
	if lArgs == 1 {
		c.branchName = args[0]
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

// getAPI returns the API that supplies methods
// required to execute this command.
func (c *diffCommand) getAPI() (DiffCommandAPI, error) {
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
func (c *diffCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	// Partially apply our time format
	formatTime := func(t time.Time) string {
		return common.FormatTime(&t, c.isoTime)
	}

	deltas, err := client.BranchInfo(c.branchName, c.detailed, formatTime)
	if err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(c.out.Write(ctx, deltas))
}
