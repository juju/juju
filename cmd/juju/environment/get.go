// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environment

import (
	"fmt"
	"strings"

	"github.com/juju/cmd"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/envcmd"
)

// GetCommand is able to output either the entire environment or
// the requested value in a format of the user's choosing.
type GetCommand struct {
	envcmd.EnvCommandBase
	api GetEnvironmentAPI
	key string
	out cmd.Output
}

const getEnvHelpDoc = `
If no extra args passed on the command line, all configuration keys and values
for the environment are output using the selected formatter.

A single environment value can be output by adding the environment key name to
the end of the command line.

Example:
  
  juju environment get default-series  (returns the default series for the environment)
`

func (c *GetCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "get",
		Args:    "[<environment key>]",
		Purpose: "view environment values",
		Doc:     strings.TrimSpace(getEnvHelpDoc),
	}
}

func (c *GetCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters)
}

func (c *GetCommand) Init(args []string) (err error) {
	c.key, err = cmd.ZeroOrOneArgs(args)
	return
}

type GetEnvironmentAPI interface {
	Close() error
	EnvironmentGet() (map[string]interface{}, error)
}

func (c *GetCommand) getAPI() (GetEnvironmentAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	return c.NewAPIClient()
}

func (c *GetCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	attrs, err := client.EnvironmentGet()
	if err != nil {
		return err
	}

	if c.key != "" {
		if value, found := attrs[c.key]; found {
			return c.out.Write(ctx, value)
		}
		return fmt.Errorf("key %q not found in %q environment.", c.key, attrs["name"])
	}
	// If key is empty, write out the whole lot.
	return c.out.Write(ctx, attrs)
}
