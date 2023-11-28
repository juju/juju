// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"fmt"
	"io"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	apiclient "github.com/juju/juju/api/client/client"
	"github.com/juju/juju/api/client/modelconfig"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/constraints"
)

// getConstraintsDoc is multi-line since we need to use ` to denote
// commands for ease in markdown.
const getConstraintsDoc = "" +
	"Shows constraints that have been set on the model with\n" +
	"`juju set-model-constraints.`\n" +
	"By default, the model is the current model.\n" +
	"Model constraints are combined with constraints set on an application\n" +
	"with `juju set-constraints` for commands (such as 'deploy') that provision\n" +
	"machines/containers for applications. Where model and application constraints overlap, the\n" +
	"application constraints take precedence.\n" +
	"Constraints for a specific application can be viewed with `juju get-constraints`.\n" + getConstraintsDocExamples

const getConstraintsDocExamples = `
Examples:

    juju get-model-constraints
    juju get-model-constraints -m mymodel

See also:
    models
    get-constraints
    set-constraints
    set-model-constraints
`

// setConstraintsDoc is multi-line since we need to use ` to denote
// commands for ease in markdown.
const setConstraintsDoc = "" +
	"Sets constraints on the model that can be viewed with\n" +
	"`juju get-model-constraints`.  By default, the model is the current model.\n" +
	"Model constraints are combined with constraints set for an application with\n" +
	"`juju set-constraints` for commands (such as 'deploy') that provision\n" +
	"machines/containers for applications. Where model and application constraints overlap, the\n" +
	"application constraints take precedence.\n" +
	"Constraints for a specific application can be viewed with `juju get-constraints`.\n" + setConstraintsDocExamples

const setConstraintsDocExamples = `
Examples:

    juju set-model-constraints cores=8 mem=16G
    juju set-model-constraints -m mymodel root-disk=64G

See also:
    models
    get-model-constraints
    get-constraints
    set-constraints
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
	return jujucmd.Info(&cmd.Info{
		Name:    "get-model-constraints",
		Purpose: "Displays machine constraints for a model.",
		Doc:     getConstraintsDoc,
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
	if client.BestAPIVersion() > 2 {
		return client, nil
	}
	return apiclient.NewClient(root, logger), nil
}

func formatConstraints(writer io.Writer, value interface{}) error {
	fmt.Fprint(writer, value.(constraints.Value).String())
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
		Name:    "set-model-constraints",
		Args:    "<constraint>=<value> ...",
		Purpose: "Sets machine constraints on a model.",
		Doc:     setConstraintsDoc,
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
	if client.BestAPIVersion() > 2 {
		return client, nil
	}
	return apiclient.NewClient(root, logger), nil
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
