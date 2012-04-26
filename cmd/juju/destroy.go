package main

import (
	"launchpad.net/gnuflag"
	"launchpad.net/juju/go/cmd"
	"launchpad.net/juju/go/juju"
)

// DestroyCommand destroys an environment.
type DestroyCommand struct {
	EnvName string
}

func (c *DestroyCommand) Info() *cmd.Info {
	return &cmd.Info{
		"destroy-environment", "[options]",
		"terminate all machines and other associated resources for an environment",
		"",
	}
}

func (c *DestroyCommand) Init(f *gnuflag.FlagSet, args []string) error {
	addEnvironFlags(&c.EnvName, f)
	if err := f.Parse(true, args); err != nil {
		return err
	}
	if err := cmd.CheckEmpty(f.Args()); err != nil {
		return err
	}
	return nil
}

func (c *DestroyCommand) Run(_ *cmd.Context) error {
	conn, err := juju.NewConn(c.EnvName)
	if err != nil {
		return err
	}
	return conn.Destroy()
}
