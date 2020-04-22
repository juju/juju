// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"time"

	"github.com/juju/cmd"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
)

// ModelCommand is a cmd.Command responsible for running a model agent.
type ModelCommand struct {
	cmd.CommandBase
}

// Info implements Command
func (m *ModelCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "model",
		Purpose: "run a juju model operator",
	})
}

// Init initializers the command for running
func (m *ModelCommand) Init(_ []string) error {
	return nil
}

// NewModelCommand creates a new ModelCommand instance properly initialized
func NewModelCommand() *ModelCommand {
	return &ModelCommand{}
}

// Run implements Command
func (m *ModelCommand) Run(ctx *cmd.Context) error {
	for {
		time.Sleep(time.Second * 30)
	}
}

// SetFlags implements Command
func (m *ModelCommand) SetFlags(f *gnuflag.FlagSet) {
}
