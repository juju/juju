// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/api/modelgeneration"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/model"
)

const (
	addGenerationSummary = "Adds a new generation to the model."
	addGenerationDoc     = `
Users need to be able to roll changes to applications in a safe guided 
processes that controls the flow such that not all units of an HA application 
are hit at once. This also allows some manual canary testing and provides 
control over the flow of changes out to the model. 

Examples:
    juju add-generation

See also:
    advance-generation
    cancel-generation
    switch-generation
`
)

// NewAddGenerationCommand wraps addGenerationCommand with sane model settings.
func NewAddGenerationCommand() cmd.Command {
	return modelcmd.Wrap(&addGenerationCommand{})
}

// addGenerationCommand is the simplified command for accessing and setting
// attributes related to adding model generations.
type addGenerationCommand struct {
	modelcmd.ModelCommandBase

	api AddGenerationCommandAPI
}

// AddGenerationCommandAPI defines an API interface to be used during testing.
//go:generate mockgen -package mocks -destination ./mocks/addgeneration_mock.go github.com/juju/juju/cmd/juju/model AddGenerationCommandAPI
type AddGenerationCommandAPI interface {
	Close() error
	AddGeneration(string) error
}

// Info implements part of the cmd.Command interface.
func (c *addGenerationCommand) Info() *cmd.Info {
	info := &cmd.Info{
		Name:    "add-generation",
		Purpose: addGenerationSummary,
		Doc:     addGenerationDoc,
	}
	return jujucmd.Info(info)
}

// SetFlags implements part of the cmd.Command interface.
func (c *addGenerationCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
}

// Init implements part of the cmd.Command interface.
func (c *addGenerationCommand) Init(args []string) error {
	if len(args) != 0 {
		return errors.Errorf("No arguments allowed")
	}
	return nil
}

// getAPI returns the API. This allows passing in a test AddGenerationCommandAPI
// implementation.
func (c *addGenerationCommand) getAPI() (AddGenerationCommandAPI, error) {
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
func (c *addGenerationCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	_, modelDetails, err := c.ModelCommandBase.ModelDetails()
	if err != nil {
		return errors.Annotate(err, "getting model details")
	}

	if err = client.AddGeneration(modelDetails.ModelUUID); err != nil {
		return err
	}

	// Now update the model store with the 'next' generation for this
	// model.
	if err = c.SetModelGeneration(model.GenerationNext); err != nil {
		return err
	}

	ctx.Stdout.Write([]byte("target generation set to next\n"))
	return nil
}
