// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"github.com/juju/cmd"
	errors "github.com/juju/errors"
	"github.com/juju/names"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/api/actions"
	"github.com/juju/juju/cmd/envcmd"
)

// ActionsCommand lists available actions for a service.
type ActionsCommand struct {
	envcmd.EnvCommandBase
	ServiceTag names.ServiceTag
	out        cmd.Output
}

const actionsDoc = `
List the actions specified for a service, along with their parameters.

For more information, see also the do command, which executes actions.
`

func (c *ActionsCommand) NewActionsClient() (*actions.Client, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, err
	}
	return actions.NewClient(root), nil
}

func (c *ActionsCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "actions",
		Args:    "<service name>",
		Purpose: "list actions specified for a service",
		Doc:     actionsDoc,
	}
}

// Init validates the service name, and makes sure only service name is passed.
func (c *ActionsCommand) Init(args []string) error {
	switch len(args) {
	case 0:
		return errors.New("no service name specified")
	case 1:
		svcName := args[0]
		if !names.IsValidService(svcName) {
			return errors.Errorf("invalid service name %q", svcName)
		}
		c.ServiceTag = names.NewServiceTag(svcName)
		return nil
	default:
		return cmd.CheckEmpty(args[1:])
	}
}

// Set up the YAML output.
func (c *ActionsCommand) SetFlags(f *gnuflag.FlagSet) {
	// TODO(binary132) add json output?
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
	})
}

// Run grabs the Actions spec from the api.  It then sets up a sensible
// output format for the map.
func (c *ActionsCommand) Run(ctx *cmd.Context) error {
	api, err := c.NewActionsClient()
	if err != nil {
		return err
	}
	defer api.Close()
	actions, err := api.ServiceCharmActions(c.ServiceTag)
	if err != nil {
		return err
	}
	actionSpecs := actions.ActionSpecs
	return c.out.Write(ctx, actionSpecs)
}
