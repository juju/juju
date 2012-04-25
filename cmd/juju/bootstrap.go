package main

import (
	"launchpad.net/gnuflag"
	"launchpad.net/juju/go/cmd"
	"launchpad.net/juju/go/juju"
)

// BootstrapCommand is responsible for launching the first machine in a juju
// environment, and setting up everything necessary to continue working.
type BootstrapCommand struct {
	envName string
	Conn    *juju.Conn
}

func (c *BootstrapCommand) Info() *cmd.Info {
	return &cmd.Info{
		"bootstrap", "[options]",
		"start up an environment from scratch",
		"",
	}
}

func (c *BootstrapCommand) Init(f *gnuflag.FlagSet, args []string) error {
	f.StringVar(&c.Environment, "e", "", "juju environment to operate in")
	f.StringVar(&c.Environment, "environment", "", "juju environment to operate in")
	if err := f.Parse(true, args); err != nil {
		return err
	}
	return cmd.CheckEmpty(f.Args())
}

// Run connects to the environment specified on the command line and bootstraps
// a juju in that environment if none already exists.
func (c *BootstrapCommand) Run(_ *cmd.Context) error {
	conn, err := juju.NewConn(c.Environment)
	if err != nil {
		return err
	}
	return cmd.CheckEmpty(args)
}

func (c *BootstrapCommand) Run() (err error) {
	return c.Conn.Bootstrap()
}
