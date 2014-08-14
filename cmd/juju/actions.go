// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/envcmd"
)

// ActionsCommand lists available actions for a service
type ActionsCommand struct {
	envcmd.EnvCommandBase
	unit string
	out  cmd.Output
}

func (c *ActionsCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "actions",
		Args:    "<unit-name>",
		Purpose: "get actions available on unit described by <unit-name>",
	}
}

func (c *ActionsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{"yaml": cmd.FormatYaml})
}

func (c *ActionsCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.New("no unit name specified")
	}
	c.unit = args[0]
	return cmd.CheckEmpty(args[1:])
}

// Run lists the available actions of the service and formats
// the result as a YAML string.
func (c *ActionsCommand) Run(ctx *cmd.Context) error {
	client, err := c.NewAPIClient()
	if err != nil {
		return err
	}
	defer client.Close()

	results, err := client.Actions(c.unit)
	if err != nil {
		return err
	}

	return c.out.Write(ctx, results)
}
