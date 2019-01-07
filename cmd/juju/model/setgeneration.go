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
)

const (
	setGenerationSummary = "Sets the generation for units and/or applications."
	setGenerationDoc     = `
Users need to be able to roll changes to applications in a safe guided 
processes that controls the flow such that not all units of an HA application 
are hit at once. This also allows some manual canary testing and provides 
control over the flow of changes out to the model. 

Examples:
    juju set-generation redis
    juju set-generation redis/0
    juju set-generation redis/0 mysql

See also:
    add-generation
    cancel-generation
    switch-generation
`
)

// NewSetGenerationCommand wraps setGenerationCommand with sane model settings.
func NewSetGenerationCommand() cmd.Command {
	return modelcmd.Wrap(&setGenerationCommand{})
}

// setGenerationCommand is the simplified command for accessing and setting
// attributes related to adding model generations.
type setGenerationCommand struct {
	modelcmd.ModelCommandBase

	api      SetGenerationCommandAPI
	entities []string
}

// SetGenerationCommandAPI defines an API interface to be used during testing.
//go:generate mockgen -package model_test -destination ./setgenerationmock_test.go github.com/juju/juju/cmd/juju/model SetGenerationCommandAPI
type SetGenerationCommandAPI interface {
	Close() error
	SetGeneration([]string) error
}

// Info implements part of the cmd.Command interface.
func (c *setGenerationCommand) Info() *cmd.Info {
	info := &cmd.Info{
		Name:    "set-generation",
		Purpose: setGenerationSummary,
		Doc:     setGenerationDoc,
	}
	return jujucmd.Info(info)
}

// SetFlags implements part of the cmd.Command interface.
func (c *setGenerationCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
}

// Init implements part of the cmd.Command interface.
func (c *setGenerationCommand) Init(args []string) error {
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

// getAPI returns the API. This allows passing in a test SetGenerationCommandAPI
// implementation.
func (c *setGenerationCommand) getAPI() (SetGenerationCommandAPI, error) {
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
func (c *setGenerationCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	return client.SetGeneration(c.entities)
}
