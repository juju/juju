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
	switchGenerationSummary = "Switch to the given generation."
	switchGenerationDoc     = `
Switch to the given generation, either current or next. 

Examples:
    juju switch-generation

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

	api SwitchGenerationCommandAPI

	generation string
}

// SwitchGenerationCommandAPI defines an API interface to be used during testing.
//go:generate mockgen -package model_test -destination ./switchgenerationmock_test.go github.com/juju/juju/cmd/juju/model SwitchGenerationCommandAPI
type SwitchGenerationCommandAPI interface {
	Close() error
	SwitchGeneration(string) error
}

// Info implements part of the cmd.Command interface.
func (c *switchGenerationCommand) Info() *cmd.Info {
	info := &cmd.Info{
		Args:    "<current|next>",
		Name:    "switch-generation",
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
		return errors.Errorf("Must specify 'current' or 'next'")
	}

	if args[0] != "current" && args[0] != "next" {
		return errors.Errorf("Must specify 'current' or 'next'")
	}

	c.generation = args[0]
	return nil
}

// getAPI returns the API. This allows passing in a test SwitchGenerationCommandAPI
// implementation.
func (c *switchGenerationCommand) getAPI() (SwitchGenerationCommandAPI, error) {
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
func (c *switchGenerationCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	// TODO (hml) 20-12-2018
	// update to check err when SwitchGeneration() is implemented in the
	// apiserver.
	client.SwitchGeneration(c.generation)

	msg := fmt.Sprintf("changes dropped and target generation set to %s\n", c.generation)
	ctx.Stdout.Write([]byte(msg))
	return nil
}
