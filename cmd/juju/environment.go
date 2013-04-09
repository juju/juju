package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
)

// GetEnvironmentCommand is able to output either the entire environment or
// the requested value in a format of the user's choosing.
type GetEnvironmentCommand struct {
	EnvCommandBase
	key string
	out cmd.Output
}

func (c *GetEnvironmentCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "get-environment",
		Args:    "[<environment key>]",
		Purpose: "view environment values",
	}
}

func (c *GetEnvironmentCommand) SetFlags(f *gnuflag.FlagSet) {
	c.EnvCommandBase.SetFlags(f)
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters)
}

func (c *GetEnvironmentCommand) Init(args []string) (err error) {
	c.key, err = c.ZeroOrOneArgs(args)
	return
}

func (c *GetEnvironmentCommand) Run(ctx *cmd.Context) error {
	conn, err := juju.NewConnFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer conn.Close()

	config := conn.Environ.Config()
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
