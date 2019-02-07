// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/modelgeneration"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/model"
)

const (
	advanceGenerationSummary = "Advances units and/or applications to the next generation."
	advanceGenerationDoc     = `
Users need to be able to roll changes to applications in a safe guided 
processes that controls the flow such that not all units of an HA application 
are hit at once. This also allows some manual canary testing and provides 
control over the flow of changes out to the model. 

Examples:
    juju advance-generation redis
    juju advance-generation redis/0
    juju advance-generation redis/0 mysql

See also:
    add-generation
    cancel-generation
    switch-generation

Aliases:
    advance
`
)

// NewAdvanceGenerationCommand wraps advanceGenerationCommand with sane model settings.
func NewAdvanceGenerationCommand() cmd.Command {
	return modelcmd.Wrap(&advanceGenerationCommand{})
}

// advanceGenerationCommand is the simplified command for accessing and setting
// attributes related to adding model generations.
type advanceGenerationCommand struct {
	modelcmd.ModelCommandBase

	api      AdvanceGenerationCommandAPI
	entities []string
}

// AdvanceGenerationCommandAPI defines an API interface to be used during testing.
//go:generate mockgen -package mocks -destination ./mocks/advancegeneration_mock.go github.com/juju/juju/cmd/juju/model AdvanceGenerationCommandAPI
type AdvanceGenerationCommandAPI interface {
	Close() error
	AdvanceGeneration(string, []string) error
}

// Info implements part of the cmd.Command interface.
func (c *advanceGenerationCommand) Info() *cmd.Info {
	info := &cmd.Info{
		Name:    "advance-generation",
		Aliases: []string{"advance"},
		Purpose: advanceGenerationSummary,
		Doc:     advanceGenerationDoc,
	}
	return jujucmd.Info(info)
}

// SetFlags implements part of the cmd.Command interface.
func (c *advanceGenerationCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
}

// Init implements part of the cmd.Command interface.
func (c *advanceGenerationCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.Errorf("unit and/or application names(s) must be specified")
	}
	for _, arg := range args {
		if !names.IsValidApplication(arg) && !names.IsValidUnit(arg) {
			return errors.Errorf("invalid application or unit name %q", arg)
		}
	}
	c.entities = args
	return nil
}

// getAPI returns the API. This allows passing in a test AdvanceGenerationCommandAPI
// implementation.
func (c *advanceGenerationCommand) getAPI() (AdvanceGenerationCommandAPI, error) {
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
func (c *advanceGenerationCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer client.Close()
	_, modelDetails, err := c.ModelDetails()
	if err != nil {
		return errors.Annotate(err, "getting model details")
	}

	if err := client.AdvanceGeneration(modelDetails.ModelUUID, c.entities); err != nil {
		return errors.Trace(err)
	}

	// Now update the model store with the 'current' generation for this
	// model.
	if err = c.SetModelGeneration(model.GenerationCurrent); err != nil {
		return err
	}

	return nil
}
