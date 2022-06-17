// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/utils/v3"
)

// AddServiceLocatorCommand implements the locator-add command.
type AddServiceLocatorCommand struct {
	cmd.CommandBase
	ctx    Context
	Labels string

	Id   string
	Name string
	Type string

	out cmd.Output
}

// NewAddServiceLocatorCommand generates a new AddServiceLocatorCommand.
func NewAddServiceLocatorCommand(ctx Context) (cmd.Command, error) {
	return &AddServiceLocatorCommand{ctx: ctx}, nil
}

// Info returns the command info structure for the locator-add command.
func (c *AddServiceLocatorCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "locator-add",
		Args:    "<locator-name>",
		Purpose: "add service locator",
	})
}

// Init parses the command's parameters.
func (c *AddServiceLocatorCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("no arguments specified")
	}
	c.Name = args[0]
	if c.Name == "" {
		return errors.Errorf("no service locator name specified")
	}
	return cmd.CheckEmpty(args[1:])
}

// Run adds metrics to the hook context.
func (c *AddServiceLocatorCommand) Run(ctx *cmd.Context) (err error) {
	// Generate new UUID for service locator
	uuid, err := utils.NewUUID()
	if err != nil {
		return errors.Annotate(err, "failed to generate new uuid for service locator")
	}
	c.Id = uuid.String()
	c.Type = "l4-service" // TODO(anvial): remove hardcode after locators assertions will be impl

	// Record new service locator
	err = c.ctx.AddServiceLocator(c.Id, c.Name, c.Type)
	if err != nil {
		return errors.Annotate(err, "cannot record service locator")
	}
	return nil
}
