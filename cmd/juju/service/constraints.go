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

const getConstraintsDoc = `
Shows the list of constraints that have been set on the specified service
using juju service set-constraints.  You can also view constraints
set for a model by using juju model get-constraints.

Constraints set on a service are combined with model constraints for
commands (such as juju deploy) that provision machines for services.  Where
model and service constraints overlap, the service constraints take
precedence.

Example:

    get-constraints wordpress

See Also:
   juju help constraints
   juju help set-constraints
   juju help deploy
   juju help machine add
   juju help add-unit
`

const setConstraintsDoc = `
Sets machine constraints on specific service, which are used as the
default constraints for all new machines provisioned by that service.
You can also set constraints on a model by using
juju model set-constraints.

Constraints set on a service are combined with model constraints for
commands (such as juju deploy) that provision machines for services.  Where
model and service constraints overlap, the service constraints take
precedence.

Example:

    set-constraints wordpress mem=4G     (all new wordpress machines must have at least 4GB of RAM)

See Also:
   juju help constraints
   juju help get-constraints
   juju help deploy
   juju help machine add
   juju help add-unit
`

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
		Purpose: "view constraints on a service",
		Doc:     getConstraintsDoc,
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
		Args:    "<service> [key=[value] ...]",
		Purpose: "set constraints on a service",
		Doc:     setConstraintsDoc,
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
