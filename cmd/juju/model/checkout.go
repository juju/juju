// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"fmt"

	"github.com/juju/juju/core/model"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/api/modelgeneration"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
)

const (
	checkoutSummary = "Work on the supplied branch."
	checkoutDoc     = `
Switch to the supplied branch, causing changes to charm configuration,
charm URL or resources to apply only to units tracking the branch.
Changing the branch to "master" causes changes to be applied to all units
as usual.

Examples:
    juju checkout test-branch
    juju checkout master

See also:
    branch
    track
    commit
    abort
    diff
`
)

// NewCheckoutCommand wraps checkoutCommand with sane model settings.
func NewCheckoutCommand() cmd.Command {
	return modelcmd.Wrap(&checkoutCommand{})
}

// checkoutCommand supplies the "checkout" CLI command used to switch the
// active branch for this operator.
type checkoutCommand struct {
	modelcmd.ModelCommandBase

	api CheckoutCommandAPI

	branchName string
}

// CheckoutCommandAPI describes API methods required
// to execute the checkout command.
//go:generate mockgen -package mocks -destination ./mocks/checkout_mock.go github.com/juju/juju/cmd/juju/model CheckoutCommandAPI
type CheckoutCommandAPI interface {
	Close() error
	HasActiveBranch(string, string) (bool, error)
}

// Info implements part of the cmd.Command interface.
func (c *checkoutCommand) Info() *cmd.Info {
	info := &cmd.Info{
		Name:    "checkout",
		Args:    "<branch name>",
		Purpose: checkoutSummary,
		Doc:     checkoutDoc,
	}
	return jujucmd.Info(info)
}

// SetFlags implements part of the cmd.Command interface.
func (c *checkoutCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
}

// Init implements part of the cmd.Command interface.
func (c *checkoutCommand) Init(args []string) error {
	if len(args) != 1 {
		return errors.Errorf("must specify a branch name to switch to")
	}
	c.branchName = args[0]
	return nil
}

// getAPI returns the API that supplies methods
// required to execute this command.
func (c *checkoutCommand) getAPI() (CheckoutCommandAPI, error) {
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
func (c *checkoutCommand) Run(ctx *cmd.Context) error {
	// If the active branch is not being set to the (default) master,
	// then first ensure that a branch with the supplied name exists.
	if c.branchName != model.GenerationMaster {
		client, err := c.getAPI()
		if err != nil {
			return err
		}
		defer func() { _ = client.Close() }()

		_, modelDetails, err := c.ModelDetails()
		if err != nil {
			return errors.Annotate(err, "getting model details")
		}
		hasBranch, err := client.HasActiveBranch(modelDetails.ModelUUID, c.branchName)
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
