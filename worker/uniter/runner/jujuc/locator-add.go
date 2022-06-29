// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/utils/v3/keyvalues"
)

// ServiceLocator represents a single metric set by the charm.
type ServiceLocator struct {
	Type   string
	Name   string
	Params map[string]interface{}
}

// LocatorAddCommand implements the locator-add command.
type LocatorAddCommand struct {
	cmd.CommandBase
	ctx    Context
	Labels string

	Id         string
	Name       string
	Type       string
	Params     map[string]interface{}
	paramsFile cmd.FileVar

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
		Args:    "<locator-type> <locator-name> key=value [key=value ...]",
		Purpose: "add service locator",
		Doc: `
locator-add adds the service locator, specified by type, name and params.
`,
	})
}

// SetFlags is part of the cmd.Command interface.
func (c *LocatorAddCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters.Formatters())
	f.StringVar(&c.ConsumerUnitId, "u", "", "specify a unit by id")
	f.StringVar(&c.ConsumerUnitId, "unit", "", "")

	f.IntVar(&c.ConsumerRelationId, "r", -1, "specify a relation by id")
	f.IntVar(&c.ConsumerRelationId, "relation", -1, "")
}

func (c *LocatorAddCommand) convertParamsFromArgs(source map[string]string) map[string]interface{} {
	params := make(map[string]interface{}, len(source))
	for k, v := range source {
		params[k] = v
	}
	return params
}

// Init parses the command's parameters.
func (c *LocatorAddCommand) Init(args []string) error {
	if len(args) < 2 {
		return errors.New("no arguments specified")
	}
	c.Type = args[0]
	c.Name = args[1]
	if c.Name == "" {
		return errors.Errorf("no service locator name specified")
	}
	params, err := keyvalues.Parse(args[2:], true)
	if err != nil {
		return errors.Trace(err)
	}
	c.Params = c.convertParamsFromArgs(params)
	return nil
}

// Run adds service locators to the hook context.
func (c *LocatorAddCommand) Run(ctx *cmd.Context) error {
	// Record new service locator
	result, err := c.ctx.AddServiceLocator(params.AddServiceLocators{
		ServiceLocators: []params.AddServiceLocatorParams{{
			Name:               c.Name,
			Type:               c.Type,
			UnitId:             c.ctx.UnitName(),
			ConsumerUnitId:     c.ConsumerUnitId,
			ConsumerRelationId: c.ConsumerRelationId,
			Params:             c.Params,
		}},
	})
	if err != nil {
		return errors.Annotate(err, "cannot record service locator")
	}

	return c.out.Write(ctx, result.Results[0].Result)
}
