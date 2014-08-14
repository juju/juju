// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"errors"

	"github.com/juju/cmd"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/envcmd"
)

// DoCommand lists available actions for a service
type DoCommand struct {
	envcmd.EnvCommandBase
	unit   string
	action string
	out    cmd.Output
}

func (c *DoCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "do",
		Purpose: "initiate named action",
	}
}

func (c *DoCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{"yaml": cmd.FormatYaml})
}

func (c *DoCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.New("no unit name specified")
	}
	if len(args) == 1 {
		return errors.New("no action name specified")
	}
	c.unit = args[0]
	c.action = args[1]
	return cmd.CheckEmpty(args[2:])
}

// Run lists the available actions of the service and formats
// the result as a YAML string.
func (c *DoCommand) Run(ctx *cmd.Context) error {
	client, err := c.NewAPIClient()
	if err != nil {
		return err
	}
	defer client.Close()

	results, err := client.Do(c.unit, c.action)
	if results != nil {
		c.out.Write(ctx, results)
	}

	// want the output regardless of error state if possible
	return err
}
