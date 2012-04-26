package main

import (
	"launchpad.net/gnuflag"
	"launchpad.net/juju/go/cmd"
	"launchpad.net/juju/go/juju"
)

// DestroyCommand destroys an environment.
type DestroyCommand struct {
	Conn *juju.Conn
}

func (c *DestroyCommand) Info() *cmd.Info {
	return &cmd.Info{
		"destroy-environment", "[options]",
		"terminate all machines and other associated resources for an environment",
		"",
	}
}

func (c *DestroyCommand) Init(f *gnuflag.FlagSet, args []string) error {
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

func (c *DestroyCommand) Run(_ *cmd.Context) error {
	return c.Conn.Destroy()
}
