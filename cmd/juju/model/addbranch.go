// Copyright 2018 Canonical Ltd.
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
	addBranchSummary = "Adds a new branch to the model."
	addBranchDoc     = `
A branch is a mechanism by which changes can be applied to units gradually at 
the operator's discretion. When changes are made to charm configuration under 
a branch, only units set to track the branch will realise such changes. 
Once the changes are assessed and deemed acceptable, the branch can be 
committed, applying the changes to the model and affecting all units.
The branch name "master" is reserved for primary model-based settings and is
not valid for new branches.

Examples:
    juju add-branch upgrade-postgresql

See also:
    track
    branch
    commit
    abort
    diff
`
)

// NewAddBranchCommand wraps addBranchCommand with sane model settings.
func NewAddBranchCommand() cmd.Command {
	return modelcmd.Wrap(&addBranchCommand{})
}

// addBranchCommand supplies the "add-branch" CLI command used to add a new branch to
// the current model.
type addBranchCommand struct {
	modelcmd.ModelCommandBase

	api AddBranchCommandAPI

	branchName string
}

// AddBranchCommandAPI describes API methods required
// to execute the branch command.
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination ./mocks/addbranch_mock.go github.com/juju/juju/cmd/juju/model AddBranchCommandAPI
type AddBranchCommandAPI interface {
	Close() error

	// AddBranch adds a new branch to the model.
	AddBranch(branchName string) error
}

// Info implements part of the cmd.Command interface.
func (c *addBranchCommand) Info() *cmd.Info {
	info := &cmd.Info{
		Name:    "add-branch",
		Args:    "<branch name>",
		Purpose: addBranchSummary,
		Doc:     addBranchDoc,
	}
	return jujucmd.Info(info)
}

// SetFlags implements part of the cmd.Command interface.
func (c *addBranchCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
}

// Init implements part of the cmd.Command interface.
func (c *addBranchCommand) Init(args []string) error {
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
func (c *addBranchCommand) getAPI() (AddBranchCommandAPI, error) {
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
func (c *addBranchCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	if err = client.AddBranch(c.branchName); err != nil {
		return err
	}

	// Update the model store with the new active branch for this model.
	if err = c.SetActiveBranch(c.branchName); err != nil {
		return err
	}

	_, err = ctx.Stdout.Write([]byte(fmt.Sprintf("Created branch %q and set active\n", c.branchName)))
	return err
}
