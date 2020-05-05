// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"fmt"
	"io"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/api/modelgeneration"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/model"
)

const (
	branchSummary = "Work on the supplied branch."
	branchDoc     = `
Switch to the supplied branch, causing changes to charm configuration to apply 
only to units tracking the branch. Changing the branch to "master" causes 
subsequent changes to be applied to all units that are not tracking an active
branch.  If no branch is supplied, active branch is displayed.

Examples:

Switch to make changes to test-branch:

    juju branch test-branch

Switch to make changes to master, changes applied to all units not tracking an active branch:

    juju branch master

Display the active branch:

    juju branch

See also:
    add-branch
    track
    commit
    abort
    diff
`
)

// NewBranchCommand wraps branchCommand with sane model settings.
func NewBranchCommand() cmd.Command {
	return modelcmd.Wrap(&branchCommand{})
}

// branchCommand supplies the "branch" CLI command used to switch the
// active branch for this operator.
type branchCommand struct {
	modelcmd.ModelCommandBase

	api BranchCommandAPI

	branchName string
}

// BranchCommandAPI describes API methods required
// to execute the branch command.
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination ./mocks/branch_mock.go github.com/juju/juju/cmd/juju/model BranchCommandAPI
type BranchCommandAPI interface {
	Close() error
	// HasActiveBranch returns true if the model has an
	// "in-flight" branch with the input name.
	HasActiveBranch(branchName string) (bool, error)
}

// Info implements part of the cmd.Command interface.
func (c *branchCommand) Info() *cmd.Info {
	info := &cmd.Info{
		Name:    "branch",
		Args:    "[<branch name>]",
		Purpose: branchSummary,
		Doc:     branchDoc,
	}
	return jujucmd.Info(info)
}

// SetFlags implements part of the cmd.Command interface.
func (c *branchCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
}

// Init implements part of the cmd.Command interface.
func (c *branchCommand) Init(args []string) error {
	if len(args) > 1 {
		return errors.Errorf("must specify a branch name to switch to or leave blank")
	}
	if len(args) == 1 {
		c.branchName = args[0]
	}
	return nil
}

// getAPI returns the API that supplies methods
// required to execute this command.
func (c *branchCommand) getAPI() (BranchCommandAPI, error) {
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

// Run (cmd.Command) sets the active branch in the local store.
func (c *branchCommand) Run(ctx *cmd.Context) error {
	// If no branch specified, print the current active branch.
	if c.branchName == "" {
		return c.activeBranch(ctx.Stdout)
	}
	// If the active branch is not being set to the (default) master,
	// then first ensure that a branch with the supplied name exists.
	if c.branchName != model.GenerationMaster {
		client, err := c.getAPI()
		if err != nil {
			return err
		}
		defer func() { _ = client.Close() }()

		hasBranch, err := client.HasActiveBranch(c.branchName)
		if err != nil {
			return errors.Annotate(err, "checking for active branch")
		}
		if !hasBranch {
			return errors.Errorf("this model has no active branch %q", c.branchName)
		}
	}

	if err := c.SetActiveBranch(c.branchName); err != nil {
		return err
	}
	msg := fmt.Sprintf("Active branch set to %q\n", c.branchName)
	_, err := ctx.Stdout.Write([]byte(msg))
	return err
}

func (c *branchCommand) activeBranch(out io.Writer) error {
	activeBranchName, err := c.ActiveBranch()
	if err != nil {
		return errors.Trace(err)
	}
	msg := fmt.Sprintf("Active branch is %q\n", activeBranchName)
	_, err = out.Write([]byte(msg))
	return err
}
