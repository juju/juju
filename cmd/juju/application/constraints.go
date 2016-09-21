// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"fmt"
	"io"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/application"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/constraints"
)

var usageGetConstraintsSummary = `
Displays machine constraints for an application.`[1:]

var usageGetConstraintsDetails = `
Shows machine constraints that have been set for an application with ` + "`juju set-\nconstraints`" + `.
By default, the model is the current model.
Application constraints are combined with model constraints, set with ` +
	"`juju \nset-model-constraints`" + `, for commands (such as 'deploy') that provision
machines for applications. Where model and application constraints overlap, the
application constraints take precedence.
Constraints for a specific model can be viewed with ` + "`juju get-model-\nconstraints`" + `.

Examples:
    juju get-constraints mysql
    juju get-constraints -m mymodel apache2

See also: 
    set-constraints
    get-model-constraints
    set-model-constraints`

var usageSetConstraintsSummary = `
Sets machine constraints for an application.`[1:]

// setConstraintsDoc is multi-line since we need to use ` to denote
// commands for ease in markdown.
var usageSetConstraintsDetails = `
Sets constraints for an application, which are used for all new machines 
provisioned for that application. They can be viewed with `[1:] + "`juju get-\nconstraints`" + `.
By default, the model is the current model.
Application constraints are combined with model constraints, set with ` +
	"`juju \nset-model-constraints`" + `, for commands (such as 'juju deploy') that 
provision machines for applications. Where model and application constraints
overlap, the application constraints take precedence.
Constraints for a specific model can be viewed with ` + "`juju get-model-\nconstraints`" + `.
This command requires that the application to have at least one unit. To apply 
constraints to
the first unit set them at the model level or pass them as an argument
when deploying.

Examples:
    juju set-constraints mysql mem=8G cores=4
    juju set-constraints -m mymodel apache2 mem=8G arch=amd64

See also: 
    get-constraints
    get-model-constraints
    set-model-constraints`

// NewServiceGetConstraintsCommand returns a command which gets application constraints.
func NewServiceGetConstraintsCommand() cmd.Command {
	return modelcmd.Wrap(&serviceGetConstraintsCommand{})
}

type serviceConstraintsAPI interface {
	Close() error
	GetConstraints(string) (constraints.Value, error)
	SetConstraints(string, constraints.Value) error
}

type serviceConstraintsCommand struct {
	modelcmd.ModelCommandBase
	ApplicationName string
	out             cmd.Output
	api             serviceConstraintsAPI
}

func (c *serviceConstraintsCommand) getAPI() (serviceConstraintsAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return application.NewClient(root), nil
}

type serviceGetConstraintsCommand struct {
	serviceConstraintsCommand
}

func (c *serviceGetConstraintsCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "get-constraints",
		Args:    "<application>",
		Purpose: usageGetConstraintsSummary,
		Doc:     usageGetConstraintsDetails,
	}
}

func formatConstraints(writer io.Writer, value interface{}) error {
	fmt.Fprint(writer, value.(constraints.Value).String())
	return nil
}

func (c *serviceGetConstraintsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	c.out.AddFlags(f, "constraints", map[string]cmd.Formatter{
		"constraints": formatConstraints,
		"yaml":        cmd.FormatYaml,
		"json":        cmd.FormatJson,
	})
}

func (c *serviceGetConstraintsCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.Errorf("no application name specified")
	}
	if !names.IsValidApplication(args[0]) {
		return errors.Errorf("invalid application name %q", args[0])
	}

	c.ApplicationName, args = args[0], args[1:]
	return cmd.CheckEmpty(args)
}

func (c *serviceGetConstraintsCommand) Run(ctx *cmd.Context) error {
	apiclient, err := c.getAPI()
	if err != nil {
		return err
	}
	defer apiclient.Close()

	cons, err := apiclient.GetConstraints(c.ApplicationName)
	if err != nil {
		return err
	}
	return c.out.Write(ctx, cons)
}

type serviceSetConstraintsCommand struct {
	serviceConstraintsCommand
	Constraints constraints.Value
}

// NewServiceSetConstraintsCommand returns a command which sets application constraints.
func NewServiceSetConstraintsCommand() cmd.Command {
	return modelcmd.Wrap(&serviceSetConstraintsCommand{})
}

func (c *serviceSetConstraintsCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "set-constraints",
		Args:    "<application> <constraint>=<value> ...",
		Purpose: usageSetConstraintsSummary,
		Doc:     usageSetConstraintsDetails,
	}
}

func (c *serviceSetConstraintsCommand) Init(args []string) (err error) {
	if len(args) == 0 {
		return errors.Errorf("no application name specified")
	}
	if !names.IsValidApplication(args[0]) {
		return errors.Errorf("invalid application name %q", args[0])
	}

	c.ApplicationName, args = args[0], args[1:]

	c.Constraints, err = constraints.Parse(args...)
	return err
}

func (c *serviceSetConstraintsCommand) Run(_ *cmd.Context) (err error) {
	apiclient, err := c.getAPI()
	if err != nil {
		return err
	}
	defer apiclient.Close()

	err = apiclient.SetConstraints(c.ApplicationName, c.Constraints)
	return block.ProcessBlockedError(err, block.BlockChange)
}
