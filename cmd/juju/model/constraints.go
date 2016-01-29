// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"github.com/juju/cmd"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/constraints"
)

const getConstraintsDoc = `
Shows a list of constraints that have been set on the model
using juju set-model-constraints.  You can also view constraints
set for a specific service by using juju get-constraints <service>.

Constraints set on a service are combined with model constraints for
commands (such as juju deploy) that provision machines for services.  Where
model and service constraints overlap, the service constraints take
precedence.

See Also:
   juju help constraints
   juju help set-model-constraints
   juju help deploy
   juju help machine add
   juju help add-unit
`

const setConstraintsDoc = `
Sets machine constraints on the model, which are used as the default
constraints for all new machines provisioned in the model (unless
overridden).  You can also set constraints on a specific service by using
juju set-constraints.

Constraints set on a service are combined with model constraints for
commands (such as juju deploy) that provision machines for services.  Where
model and service constraints overlap, the service constraints take
precedence.

Example:

   juju model set-constraints mem=8G                         (all new machines in the model must have at least 8GB of RAM)

See Also:
   juju help constraints
   juju help get-model-constraints
   juju help deploy
   juju help machine add
   juju help add-unit
`

// ConstraintsAPI defines methods on the client API that
// the get-constraints and set-constraints commands call
type ConstraintsAPI interface {
	Close() error
	GetModelConstraints() (constraints.Value, error)
	SetModelConstraints(constraints.Value) error
}

// NewModelGetConstraintsCommand returns a command to get model constraints.
func NewModelGetConstraintsCommand() cmd.Command {
	return modelcmd.Wrap(&modelGetConstraintsCommand{})
}

// modelGetConstraintsCommand shows the constraints for a model.
type modelGetConstraintsCommand struct {
	modelcmd.ModelCommandBase
	out cmd.Output
	api ConstraintsAPI
}

func (c *modelGetConstraintsCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "get-model-constraints",
		Purpose: "view constraints on the model",
		Doc:     getConstraintsDoc,
	}
}

func (c *modelGetConstraintsCommand) Init(args []string) error {
	return cmd.CheckEmpty(args)
}

func (c *modelGetConstraintsCommand) getAPI() (ConstraintsAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	return c.NewAPIClient()
}

func formatConstraints(value interface{}) ([]byte, error) {
	return []byte(value.(constraints.Value).String()), nil
}

func (c *modelGetConstraintsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "constraints", map[string]cmd.Formatter{
		"constraints": formatConstraints,
		"yaml":        cmd.FormatYaml,
		"json":        cmd.FormatJson,
	})
}

func (c *modelGetConstraintsCommand) Run(ctx *cmd.Context) error {
	apiclient, err := c.getAPI()
	if err != nil {
		return err
	}
	defer apiclient.Close()

	cons, err := apiclient.GetModelConstraints()
	if err != nil {
		return err
	}
	return c.out.Write(ctx, cons)
}

// NewModelSetConstraintsCommand returns a command to set model constraints.
func NewModelSetConstraintsCommand() cmd.Command {
	return modelcmd.Wrap(&modelSetConstraintsCommand{})
}

// modelSetConstraintsCommand sets the constraints for a model.
type modelSetConstraintsCommand struct {
	modelcmd.ModelCommandBase
	api         ConstraintsAPI
	Constraints constraints.Value
}

func (c *modelSetConstraintsCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "set-model-constraints",
		Args:    "[key=[value] ...]",
		Purpose: "set constraints on the model",
		Doc:     setConstraintsDoc,
	}
}

func (c *modelSetConstraintsCommand) Init(args []string) (err error) {
	c.Constraints, err = constraints.Parse(args...)
	return err
}

func (c *modelSetConstraintsCommand) getAPI() (ConstraintsAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	return c.NewAPIClient()
}

func (c *modelSetConstraintsCommand) Run(_ *cmd.Context) (err error) {
	apiclient, err := c.getAPI()
	if err != nil {
		return err
	}
	defer apiclient.Close()

	err = apiclient.SetModelConstraints(c.Constraints)
	return block.ProcessBlockedError(err, block.BlockChange)
}
