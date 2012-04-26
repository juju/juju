package main

import (
	"launchpad.net/gnuflag"
	"launchpad.net/juju/go/cmd"
	"launchpad.net/juju/go/juju"
)

// BootstrapCommand is responsible for launching the first machine in a juju
// environment, and setting up everything necessary to continue working.
type BootstrapCommand struct {
	Conn *juju.Conn
}

func (c *BootstrapCommand) Info() *cmd.Info {
	return &cmd.Info{
		"bootstrap", "[options]",
		"start up an environment from scratch",
		"",
	}
}

func (c *BootstrapCommand) Init(f *gnuflag.FlagSet, args []string) error {
	var envName string
	f.StringVar(&envName, "e", "", "juju environment to operate in")
	f.StringVar(&envName, "environment", "", "")
	if err := f.Parse(true, args); err != nil {
		return err
	}
	if err := cmd.CheckEmpty(f.Args()); err != nil {
		return err
	}
	conn, err := juju.NewConn(envName)
	if err != nil {
		return err
	}
	c.Conn = conn
	return nil
}

// Run connects to the environment specified on the command line and bootstraps
// a juju in that environment if none already exists.
func (c *BootstrapCommand) Run(_ *cmd.Context) error {
	return c.Conn.Bootstrap()
}
