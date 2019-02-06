// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/model"
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

	generation string
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

// Run (cmd.Command) sets the active generation in the local store.
func (c *switchGenerationCommand) Run(ctx *cmd.Context) error {
	if err := c.SetModelGeneration(model.GenerationVersion(c.generation)); err != nil {
		return err
	}
	msg := fmt.Sprintf("target generation set to %s\n", c.generation)
	_, err := ctx.Stdout.Write([]byte(msg))
	return err
}
