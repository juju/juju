// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/utils/v3"
)

// LocatorAddCommand implements the locator-add command.
type LocatorAddCommand struct {
	cmd.CommandBase
	ctx    Context
	Labels string

	Id   string
	Name string
	Type string

	ConsumerUnitId     string
	ConsumerRelationId int

	out cmd.Output
}

// NewLocatorAddCommand generates a new LocatorAddCommand.
func NewLocatorAddCommand(ctx Context) (cmd.Command, error) {
	return &LocatorAddCommand{ctx: ctx}, nil
}

// Info returns the command info structure for the locator-add command.
func (c *LocatorAddCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "locator-add",
		Args:    "<locator-name>",
		Purpose: "add service locator",
		Doc: `
locator-add adds the service locator, specified by name.

... . 
`,
	})
}

// SetFlags is part of the cmd.Command interface.
func (c *LocatorAddCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.ConsumerUnitId, "u", "", "Specify a unit by id")
	f.StringVar(&c.ConsumerUnitId, "unit", "", "")

	f.IntVar(&c.ConsumerRelationId, "r", -1, "Specify a relation by id")
	f.IntVar(&c.ConsumerRelationId, "relation", -1, "")
}

// Init parses the command's parameters.
func (c *LocatorAddCommand) Init(args []string) error {
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
func (c *LocatorAddCommand) Run(ctx *cmd.Context) (err error) {
	// Generate new UUID for service locator
	uuid, err := utils.NewUUID()
	if err != nil {
		return errors.Annotate(err, "failed to generate new uuid for service locator")
	}
	c.Id = uuid.String()
	c.Type = "l4-service" // TODO(anvial): remove hardcode after locators assertions will be implemented

	// Record new service locator
	err = c.ctx.AddServiceLocator(params.AddServiceLocators{
		ServiceLocators: []params.AddServiceLocatorParams{{
			ServiceLocatorUUID: c.Id,
			Name:               c.Name,
			Type:               c.Type,
			UnitId:             "unit/0", // TODO(anvial): remove hardcode
			ConsumerUnitId:     c.ConsumerUnitId,
			ConsumerRelationId: c.ConsumerRelationId,
			Params:             map[string]interface{}{}, // TODO(anvial): remove hardcode
		}},
	})
	if err != nil {
		return errors.Annotate(err, "cannot record service locator")
	}
	return nil
}
