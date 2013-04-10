package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
	"strings"
)

// GetEnvironmentCommand is able to output either the entire environment or
// the requested value in a format of the user's choosing.
type GetEnvironmentCommand struct {
	EnvCommandBase
	key string
	out cmd.Output
}

const getEnvHelpDoc = `
If no extra args passed on the command line, all configuration keys and values
for the environment are output using the selected formatter.

A single environment value can be output by adding the environment key name to
the end of the command line.

e.g. $ juju get-environment default-series
     precise
`

func (c *GetEnvironmentCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "get-environment",
		Args:    "[<environment key>]",
		Purpose: "view environment values",
		Doc:     strings.TrimSpace(getEnvHelpDoc),
		Aliases: []string{"get-env"},
	}
}

func (c *GetEnvironmentCommand) SetFlags(f *gnuflag.FlagSet) {
	c.EnvCommandBase.SetFlags(f)
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters)
	// TODO(thumper) --private to also include private.
}

func (c *GetEnvironmentCommand) Init(args []string) (err error) {
	c.key, err = cmd.ZeroOrOneArgs(args)
	return
}

func (c *GetEnvironmentCommand) Run(ctx *cmd.Context) error {
	conn, err := juju.NewConnFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer conn.Close()

	// Get the existing environment config from the state.
	config, err := conn.State.EnvironConfig()
	if err != nil {
		return err
	}
	attrs := config.AllAttrs()

	// If no key specified, write out the whole lot.
	if c.key == "" {
		return c.out.Write(ctx, attrs)
	}

	value, found := attrs[c.key]
	if found {
		return c.out.Write(ctx, value)
	}

	return fmt.Errorf("Environment key %q not found in %q environment.", c.key, config.Name())
}
