package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju/go/cmd"
	"launchpad.net/juju/go/juju"
)

// DestroyCommand destroys an environment.
type DestroyCommand struct {
	conn
}

func (c *DestroyCommand) Info() *cmd.Info {
	return &cmd.Info{
		"destroy-environment", "[options]",
		"start up an environment from scratch",
		"",
		true,
	}
}

func (c *DestroyCommand) ParsePositional(args []string) error {
	return cmd.CheckEmpty(args)
}

func (c *DestroyCommand) Run() error {
	return c.conn.Destroy()
}
