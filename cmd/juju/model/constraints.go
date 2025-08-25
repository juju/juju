// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"fmt"
	"io"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/api/client/modelconfig"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/constraints"
)

// getConstraintsDoc is multi-line since we need to use ` to denote
// commands for ease in markdown.
const getConstraintsDoc = `
Shows constraints that have been set on the model with ` + "`juju set-model-constraints`" + `.
By default, the model is the current model.
Model constraints are combined with constraints set on an application.
with ` + "`juju set-constraints`" + ` for commands (such as ` + "`deploy`" + `) that provision
machines/containers for applications. Where model and application constraints overlap, the
application constraints take precedence. Constraints for a specific application
can be viewed with ` + "`juju constraints`" + `.

`

const getConstraintsDocExamples = `
    juju model-constraints
    juju model-constraints -m mymodel
`

// setConstraintsDoc is multi-line since we need to use ` to denote
// commands for ease in markdown.
const setConstraintsDoc = `
Sets constraints on the model that can be viewed with ` + "`juju model-constraints`" + `.
By default, the model is the current model.
Model constraints are combined with constraints set for an application with
` + "`juju set-constraints`" + ` for commands (such as ` + "`deploy`" + `) that provision
machines/containers for applications. Where model and application constraints overlap, the
application constraints take precedence.
Constraints for a specific application can be viewed with ` + "`juju constraints`" + `.
`

const setConstraintsDocExamples = `
    juju set-model-constraints cores=8 mem=16G
    juju set-model-constraints -m mymodel root-disk=64G
`

// ConstraintsAPI defines methods on the client API that
// the constraints and set-constraints commands call
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
	return jujucmd.Info(&cmd.Info{
		Name:     "model-constraints",
		Purpose:  "Displays machine constraints for a model.",
		Doc:      getConstraintsDoc,
		Examples: getConstraintsDocExamples,
		SeeAlso: []string{
			"models",
			"constraints",
			"set-constraints",
			"set-model-constraints",
		},
	})
}

func (c *modelGetConstraintsCommand) Init(args []string) error {
	return cmd.CheckEmpty(args)
}

func (c *modelGetConstraintsCommand) getAPI() (ConstraintsAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	client := modelconfig.NewClient(root)
	return client, nil
}

func formatConstraints(writer io.Writer, value interface{}) error {
	fmt.Fprintln(writer, value.(constraints.Value).String())
	return nil
}

func (c *modelGetConstraintsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	c.out.AddFlags(f, "constraints", map[string]cmd.Formatter{
		"constraints": formatConstraints,
		"yaml":        cmd.FormatYaml,
		"json":        cmd.FormatJson,
	})
}

func (c *modelGetConstraintsCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	cons, err := client.GetModelConstraints()
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
	return jujucmd.Info(&cmd.Info{
		Name:     "set-model-constraints",
		Args:     "<constraint>=<value> ...",
		Purpose:  "Sets machine constraints on a model.",
		Doc:      setConstraintsDoc,
		Examples: setConstraintsDocExamples,
		SeeAlso: []string{
			"models",
			"model-constraints",
			"constraints",
			"set-constraints",
		},
	})
}

func (c *modelSetConstraintsCommand) Init(args []string) (err error) {
	c.Constraints, err = constraints.Parse(args...)
	return err
}

func (c *modelSetConstraintsCommand) getAPI() (ConstraintsAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	client := modelconfig.NewClient(root)
	return client, nil
}

func (c *modelSetConstraintsCommand) Run(_ *cmd.Context) (err error) {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	err = client.SetModelConstraints(c.Constraints)
	return block.ProcessBlockedError(err, block.BlockChange)
}
