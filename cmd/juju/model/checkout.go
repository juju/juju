// Copyright 2018 Canonical Ltd.
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
	switchGenerationSummary = "Switch to the given generation."
	switchGenerationDoc     = `
Switch to the given generation, either current or next. 

Examples:
    juju switch-generation next

See also:
    add-generation
    advance-generation
    cancel-generation
`
)

// NewSwitchGenerationCommand wraps switchGenerationCommand with sane model settings.
func NewSwitchGenerationCommand() cmd.Command {
	return modelcmd.Wrap(&switchGenerationCommand{})
}

// switchGenerationCommand is the simplified command for accessing and setting
// attributes related to switching model generations.
type switchGenerationCommand struct {
	modelcmd.ModelCommandBase

	api CheckoutCommandAPI

	branchName string
}

// CheckoutCommandAPI defines an API interface to be used during testing.
//go:generate mockgen -package mocks -destination ./mocks/checkout_mock.go github.com/juju/juju/cmd/juju/model CheckoutCommandAPI
type CheckoutCommandAPI interface {
	Close() error
	HasActiveBranch(string, string) (bool, error)
}

// Info implements part of the cmd.Command interface.
func (c *switchGenerationCommand) Info() *cmd.Info {
	info := &cmd.Info{
		Name:    "switch-generation",
		Args:    "<branch name>",
		Purpose: switchGenerationSummary,
		Doc:     switchGenerationDoc,
	}
	return jujucmd.Info(info)
}

// SetFlags implements part of the cmd.Command interface.
func (c *switchGenerationCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
}

// Init implements part of the cmd.Command interface.
func (c *switchGenerationCommand) Init(args []string) error {
	if len(args) != 1 {
		return errors.Errorf("must specify a branch name to switch to")
	}
	c.branchName = args[0]
	return nil
}

// getAPI returns the API. This allows passing in a test SwitchGenerationCommandAPI
// Run (cmd.Command) sets the active generation in the local store.
// implementation.
func (c *switchGenerationCommand) getAPI() (CheckoutCommandAPI, error) {
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
func (c *switchGenerationCommand) Run(ctx *cmd.Context) error {
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

	if err := c.SetModelGeneration(c.branchName); err != nil {
		return err
	}
	msg := fmt.Sprintf("target generation set to %q\n", c.branchName)
	_, err := ctx.Stdout.Write([]byte(msg))
	return err
}
