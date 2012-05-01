package main

import (
	"launchpad.net/gnuflag"
	"launchpad.net/juju/go/cmd"
	"launchpad.net/juju/go/environs"
	"launchpad.net/juju/go/juju"
)

// BootstrapCommand is responsible for launching the first machine in a juju
// environment, and setting up everything necessary to continue working.
type BootstrapCommand struct {
	EnvName string
	UploadTools bool
}

func (c *BootstrapCommand) Info() *cmd.Info {
	return &cmd.Info{"bootstrap", "", "start up an environment from scratch", ""}
}

func (c *BootstrapCommand) Init(f *gnuflag.FlagSet, args []string) error {
	addEnvironFlags(&c.EnvName, f)
	f.BoolVar(&c.UploadTools, "upload-tools", false, "upload local version of tools before bootstrapping")
	if err := f.Parse(true, args); err != nil {
		return err
	}
	return cmd.CheckEmpty(f.Args())
}

// Run connects to the environment specified on the command line and bootstraps
// a juju in that environment if none already exists.
func (c *BootstrapCommand) Run(_ *cmd.Context) error {
	conn, err := juju.NewConn(c.EnvName)
	if err != nil {
		return err
	}
	if c.UploadTools {
		if err := environs.UploadTools(conn.Environ); err != nil {
			return err
		}
	}
	return conn.Bootstrap()
}
