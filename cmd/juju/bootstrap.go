package main

import (
	"launchpad.net/juju/go/cmd"
	"launchpad.net/juju/go/juju"
	"log"
	"launchpad.net/gnuflag"
)

// BootstrapCommand is responsible for launching the first machine in a juju
// environment, and setting up everything necessary to continue working.
type BootstrapCommand struct {
	envName string
	Conn *juju.Conn
}

func (c *BootstrapCommand) Info() *cmd.Info {
	return &cmd.Info{
		"bootstrap", "[options]",
		"start up an environment from scratch",
		"",
		true,
	}
}

func (c *BootstrapCommand) InitFlagSet(f *gnuflag.FlagSet) {
	f.StringVar(&c.envName, "e", "", "juju environment to operate in")
	f.StringVar(&c.envName, "environment", "", "")
}

func (c *BootstrapCommand) ParsePositional(args []string) (err error) {
	c.Conn, err = juju.NewConn(c.envName)
	if err != nil {
		return err
	}
	return cmd.CheckEmpty(args)
}

func (c *BootstrapCommand) Run() (err error) {
log.Printf("calling conn.Bootstrap on %v\n", c.Conn)
	return c.Conn.Bootstrap()
}
