// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/api/service"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/constraints"
)

var usageGetConstraintsSummary = `
Displays machine constraints for a service.`[1:]

var usageGetConstraintsDetails = `
Shows machine constraints that have been set for a service with ` + "`juju set-\nconstraints`" + `.
By default, the model is the current model.
Service constraints are combined with model constraints, set with ` +
	"`juju \nset-model-constraints`" + `, for commands (such as 'deploy') that provision
machines for services. Where model and service constraints overlap, the
service constraints take precedence.
Constraints for a specific model can be viewed with ` + "`juju get-model-\nconstraints`" + `.

Examples:
    juju get-constraints mysql
    juju get-constraints -m mymodel apache2

See also: 
    set-constraints
    get-model-constraints
    set-model-constraints`

var usageSetConstraintsSummary = `
Sets machine constraints for a service.`[1:]

// setConstraintsDoc is multi-line since we need to use ` to denote
// commands for ease in markdown.
var usageSetConstraintsDetails = `
Sets constraints for a service, which are used for all new machines 
provisioned for that service. They can be viewed with `[1:] + "`juju get-\nconstraints`" + `.
By default, the model is the current model.
Service constraints are combined with model constraints, set with ` +
	"`juju \nset-model-constraints`" + `, for commands (such as 'juju deploy') that 
provision machines for services. Where model and service constraints
overlap, the service constraints take precedence.
Constraints for a specific model can be viewed with ` + "`juju get-model-\nconstraints`" + `.
This command requires that the service to have at least one unit. To apply 
constraints to the first unit set them at the model level or pass them as 
an argument when deploying.

Examples:
    juju set-constraints mysql mem=8G cpu-cores=4
    juju set-constraints -m mymodel apache2 mem=8G arch=amd64

See also: 
    get-constraints
    get-model-constraints
    set-model-constraints`

// NewServiceGetConstraintsCommand returns a command which gets service constraints.
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
	ServiceName string
	out         cmd.Output
	api         serviceConstraintsAPI
}

func (c *serviceConstraintsCommand) getAPI() (serviceConstraintsAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return service.NewClient(root), nil
}

type serviceGetConstraintsCommand struct {
	serviceConstraintsCommand
}

func (c *serviceGetConstraintsCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "get-constraints",
		Args:    "<service>",
		Purpose: usageGetConstraintsSummary,
		Doc:     usageGetConstraintsDetails,
	}
}

func formatConstraints(value interface{}) ([]byte, error) {
	return []byte(value.(constraints.Value).String()), nil
}

func (c *serviceGetConstraintsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "constraints", map[string]cmd.Formatter{
		"constraints": formatConstraints,
		"yaml":        cmd.FormatYaml,
		"json":        cmd.FormatJson,
	})
}

func (c *serviceGetConstraintsCommand) Init(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no service name specified")
	}
	if !names.IsValidService(args[0]) {
		return fmt.Errorf("invalid service name %q", args[0])
	}

	c.ServiceName, args = args[0], args[1:]
	return cmd.CheckEmpty(args)
}

func (c *serviceGetConstraintsCommand) Run(ctx *cmd.Context) error {
	apiclient, err := c.getAPI()
	if err != nil {
		return err
	}
	defer apiclient.Close()

	cons, err := apiclient.GetConstraints(c.ServiceName)
	if err != nil {
		return err
	}
	return c.out.Write(ctx, cons)
}

type serviceSetConstraintsCommand struct {
	serviceConstraintsCommand
	Constraints constraints.Value
}

// NewServiceSetConstraintsCommand returns a command which sets service constraints.
func NewServiceSetConstraintsCommand() cmd.Command {
	return modelcmd.Wrap(&serviceSetConstraintsCommand{})
}

func (c *serviceSetConstraintsCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "set-constraints",
		Args:    "<service> <constraint>=<value> ...",
		Purpose: usageSetConstraintsSummary,
		Doc:     usageSetConstraintsDetails,
	}
}

func (c *serviceSetConstraintsCommand) Init(args []string) (err error) {
	if len(args) == 0 {
		return fmt.Errorf("no service name specified")
	}
	if !names.IsValidService(args[0]) {
		return fmt.Errorf("invalid service name %q", args[0])
	}

	c.ServiceName, args = args[0], args[1:]

	c.Constraints, err = constraints.Parse(args...)
	return err
}

func (c *serviceSetConstraintsCommand) Run(_ *cmd.Context) (err error) {
	apiclient, err := c.getAPI()
	if err != nil {
		return err
	}
	defer apiclient.Close()

	err = apiclient.SetConstraints(c.ServiceName, c.Constraints)
	return block.ProcessBlockedError(err, block.BlockChange)
}
