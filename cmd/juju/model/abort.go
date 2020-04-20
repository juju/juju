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
	abortSummary = "Aborts a branch in the model."
	abortDoc     = `
Aborting a branch aborts changes made to that branch.  A branch
can only be aborted if no units are tracked by that branch.

Examples:
    juju abort upgrade-postgresql

See also:
    track
    branch
    commit
    add-branch
    diff
`
)

// NewAbortCommand wraps abortCommand with sane model settings.
func NewAbortCommand() cmd.Command {
	return modelcmd.Wrap(&abortCommand{})
}

// abortCommand supplies the "add-branch" CLI command used to add a new branch to
// the current model.
type abortCommand struct {
	modelcmd.ModelCommandBase

	api AbortCommandAPI

	branchName string
}

// AbortCommandAPI describes API methods required
// to execute the branch command.
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination ./mocks/abort_mock.go github.com/juju/juju/cmd/juju/model AbortCommandAPI
type AbortCommandAPI interface {
	Close() error

	// Abort aborts an existing branch to the model.
	AbortBranch(branchName string) error

	// HasActiveBranch returns true if the model has an
	// "in-flight" branch with the input name.
	HasActiveBranch(branchName string) (bool, error)
}

// Info implements part of the cmd.Command interface.
func (c *abortCommand) Info() *cmd.Info {
	info := &cmd.Info{
		Name:    "abort",
		Args:    "<branch name>",
		Purpose: abortSummary,
		Doc:     abortDoc,
	}
	return jujucmd.Info(info)
}

// SetFlags implements part of the cmd.Command interface.
func (c *abortCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
}

// Init implements part of the cmd.Command interface.
func (c *abortCommand) Init(args []string) error {
	if len(args) != 1 {
		return errors.Errorf("expected a branch name")
	}
	if err := model.ValidateBranchName(args[0]); err != nil {
		return err
	}
	c.branchName = args[0]
	return nil
}

// getAPI returns the API that supplies methods
// required to execute this command.
func (c *abortCommand) getAPI() (AbortCommandAPI, error) {
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
func (c *abortCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	hasBranch, err := client.HasActiveBranch(c.branchName)
	if err != nil {
		return err
	}
	if !hasBranch {
		return errors.Errorf("this model has no active branch %q", c.branchName)
	}

	if err = client.AbortBranch(c.branchName); err != nil {
		return err
	}

	// Update the model store with the master branch for this model.
	if err = c.SetActiveBranch(model.GenerationMaster); err != nil {
		return err
	}

	msg := fmt.Sprintf("Aborting all changes in %q and closing branch.\n", c.branchName)
	msg = msg + fmt.Sprintf("Active branch set to %q\n", model.GenerationMaster)
	_, err = ctx.Stdout.Write([]byte(msg))
	return err
}
