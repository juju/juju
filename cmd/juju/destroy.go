package main

import (
	"launchpad.net/juju/go/cmd"
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
	if err := c.InitConn(); err != nil {
		return err
	}
	return c.Conn.Destroy()
}
