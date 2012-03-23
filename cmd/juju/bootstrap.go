package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju/go/cmd"
	"launchpad.net/juju/go/juju"
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
	return c.conn.Bootstrap()
}
