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
)

const (
	branchSummary = "Adds a new branch to the model."
	branchDoc     = `
A branch is a mechanism by which changes can be applied to units gradually at 
the operator's discretion. When changes are made to charm configuration, charm
URL or resources against a branch, only units set to track the branch will realise 
such changes. Once the changes are assessed and deemed acceptable, the branch 
can be committed, applying the changes to the model and affecting all units.

Examples:
    juju branch upgrade-postgresql

See also:
    track
    checkout
    commit
    abort
	diff
`
)

// NewBranchCommand wraps branchCommand with sane model settings.
func NewBranchCommand() cmd.Command {
	return modelcmd.Wrap(&branchCommand{})
}

// branchCommand supplies the "branch" CLI command used to add a new branch to
// the current model.
type branchCommand struct {
	modelcmd.ModelCommandBase

	api BranchCommandAPI

	branchName string
}

// BranchCommandAPI describes API methods required
// to execute the branch command..
//go:generate mockgen -package mocks -destination ./mocks/branch_mock.go github.com/juju/juju/cmd/juju/model BranchCommandAPI
type BranchCommandAPI interface {
	Close() error
	AddBranch(string, string) error
}

// Info implements part of the cmd.Command interface.
func (c *branchCommand) Info() *cmd.Info {
	info := &cmd.Info{
		Name:    "branch",
		Args:    "<branch name>",
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
	if len(args) != 1 {
		return errors.Errorf("expected a branch name")
	}
	c.branchName = args[0]
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

// Run implements the meaty part of the cmd.Command interface.
func (c *branchCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	_, modelDetails, err := c.ModelCommandBase.ModelDetails()
	if err != nil {
		return errors.Annotate(err, "getting model details")
	}

	if err = client.AddBranch(modelDetails.ModelUUID, c.branchName); err != nil {
		return err
	}

	// Now update the model store with the 'next' generation for this
	// model.
	if err = c.SetModelGeneration(c.branchName); err != nil {
		return err
	}

	_, err = ctx.Stdout.Write([]byte(fmt.Sprintf("Active branch set to %q\n", c.branchName)))
	return err
}
