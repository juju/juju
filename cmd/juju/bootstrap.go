package main

import (
	"launchpad.net/juju/go/cmd"
)

// BootstrapCommand is responsible for launching the first machine in a juju
// environment, and setting up everything necessary to continue working.
type BootstrapCommand struct {
	conn
}

func (c *BootstrapCommand) Info() *cmd.Info {
	return &cmd.Info{
		"bootstrap", "[options]",
		"start up an environment from scratch",
		"",
		true,
	}
}

func (c *BootstrapCommand) ParsePositional(args []string) error {
	return cmd.CheckEmpty(args)
}

func (c *BootstrapCommand) Run() error {
	if err := c.InitConn(); err != nil {
		return err
	}
	return c.Conn.Bootstrap()
}
