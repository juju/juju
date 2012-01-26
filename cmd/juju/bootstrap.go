package main

import (
	"fmt"
	"launchpad.net/juju/go/juju"
	"launchpad.net/~rogpeppe/juju/gnuflag/flag"
)

// BootstrapCommand is responsible for launching the first machine in a juju
// environment, and setting up everything necessary to continue working.
type BootstrapCommand struct {
	Environment string
}

// Ensure Command interface.
var _ Command = (*BootstrapCommand)(nil)

// Info returns a description of BootstrapCommand.
func (c *BootstrapCommand) Info() *Info {
	return &Info{
		"bootstrap",
		"juju bootstrap [options]",
		"start up an environment from scratch",
		"",
	}
}

// InitFlagSet prepares a FlagSet for use.
func (c *BootstrapCommand) InitFlagSet(f *flag.FlagSet) {
	f.StringVar(&c.Environment, "e", "", "juju environment to operate in")
	f.StringVar(&c.Environment, "environment", "", "juju environment to operate in")
}

// ParsePositional checks that no unexpected extra command-line arguments have
// been specified.
func (c *BootstrapCommand) ParsePositional(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("unrecognised args: %s", args)
	}
	return nil
}

// Run connects to the environment specified on the command line and bootstraps
// a juju in that environment if none already exists.
func (c *BootstrapCommand) Run() error {
	conn, err := juju.NewConn(c.Environment)
	if err != nil {
		return err
	}
	return conn.Bootstrap()
}
