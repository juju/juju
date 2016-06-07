// Copyright 2014, 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"github.com/juju/cmd"
	errors "github.com/juju/errors"
	"github.com/juju/names"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
)

func NewListCommand() cmd.Command {
	return modelcmd.Wrap(&listCommand{})
}

// listCommand lists actions defined by the charm of a given service.
type listCommand struct {
	ActionCommandBase
	serviceTag names.ServiceTag
	fullSchema bool
	out        cmd.Output
}

const listDoc = `
List the actions available to run on the target service, with a short
description.  To show the full schema for the actions, use --schema.

For more information, see also the 'run-ation' command, which executes actions.
`

// Set up the output.
func (c *listCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters)
	f.BoolVar(&c.fullSchema, "schema", false, "display the full action schema")
}

func (c *listCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "actions",
		Args:    "<service name>",
		Purpose: "list actions defined for a service",
		Doc:     listDoc,
		Aliases: []string{"list-actions"},
	}
}

// Init validates the service name and any other options.
func (c *listCommand) Init(args []string) error {
	switch len(args) {
	case 0:
		return errors.New("no service name specified")
	case 1:
		svcName := args[0]
		if !names.IsValidService(svcName) {
			return errors.Errorf("invalid service name %q", svcName)
		}
		c.serviceTag = names.NewServiceTag(svcName)
		return nil
	default:
		return cmd.CheckEmpty(args[1:])
	}
}

// Run grabs the Actions spec from the api.  It then sets up a sensible
// output format for the map.
func (c *listCommand) Run(ctx *cmd.Context) error {
	api, err := c.NewActionAPIClient()
	if err != nil {
		return err
	}
	defer api.Close()

	actions, err := api.ServiceCharmActions(params.Entity{c.serviceTag.String()})
	if err != nil {
		return err
	}

	output := actions.ActionSpecs
	if len(output) == 0 {
		return c.out.Write(ctx, "No actions defined for "+c.serviceTag.Id())
	}

	if c.fullSchema {
		verboseSpecs := make(map[string]interface{})
		for k, v := range output {
			verboseSpecs[k] = v.Params
		}

		return c.out.Write(ctx, verboseSpecs)
	}

	shortOutput := make(map[string]string)
	for name, action := range actions.ActionSpecs {
		shortOutput[name] = action.Description
		if shortOutput[name] == "" {
			shortOutput[name] = "No description"
		}
	}
	return c.out.Write(ctx, shortOutput)
}
