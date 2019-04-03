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
	cancelGenerationSummary = "Cancels the new generation to the model."
	cancelGenerationDoc     = `
Cancel the next generation. This will abort anything in the next
generation and return the active target to the current generation. 

Examples:
    juju cancel-generation

See also:
    add-generation
    advance-generation
    switch-generation
`
)

// NewCancelGenerationCommand wraps cancelGenerationCommand with sane model settings.
func NewCancelGenerationCommand() cmd.Command {
	return modelcmd.Wrap(&cancelGenerationCommand{})
}

// cancelGenerationCommand is the simplified command for accessing and setting
// attributes related to canceling model generations.
type cancelGenerationCommand struct {
	modelcmd.ModelCommandBase

	api CancelGenerationCommandAPI

	branchName string
}

// CancelGenerationCommandAPI defines an API interface to be used during testing.
//go:generate mockgen -package mocks -destination ./mocks/cancelgeneration_mock.go github.com/juju/juju/cmd/juju/model CancelGenerationCommandAPI
type CancelGenerationCommandAPI interface {
	Close() error
	CommitBranch(string, string) (int, error)
}

// Info implements part of the cmd.Command interface.
func (c *cancelGenerationCommand) Info() *cmd.Info {
	info := &cmd.Info{
		Name:    "cancel-generation",
		Args:    "<branch name>",
		Purpose: cancelGenerationSummary,
		Doc:     cancelGenerationDoc,
	}
	return jujucmd.Info(info)
}

// SetFlags implements part of the cmd.Command interface.
func (c *cancelGenerationCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
}

// Init implements part of the cmd.Command interface.
func (c *cancelGenerationCommand) Init(args []string) error {
	if len(args) != 1 {
		return errors.Errorf("must specify a branch name to commit")
	}
	c.branchName = args[0]
	return nil
}

// getAPI returns the API. This allows passing in a test CancelGenerationCommandAPI
// implementation.
func (c *cancelGenerationCommand) getAPI() (CancelGenerationCommandAPI, error) {
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
func (c *cancelGenerationCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	_, modelDetails, err := c.ModelCommandBase.ModelDetails()
	if err != nil {
		return errors.Annotate(err, "getting model details")
	}

	newGenId, err := client.CommitBranch(modelDetails.ModelUUID, c.branchName)
	if err != nil {
		return err
	}

	// Set the active branch to be the master.
	if err = c.SetModelGeneration(model.GenerationMaster); err != nil {
		return err
	}

	msg := fmt.Sprintf("changes committed; model is now at generation %d\nactive branch set to %q",
		newGenId, model.GenerationMaster)

	_, err = ctx.Stdout.Write([]byte(msg))
	return err
}
