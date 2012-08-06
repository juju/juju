package main

import (
	"errors"

	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
)

// ExposeCommand is responsible exposing services.
type ExposeCommand struct {
	EnvName     string
	ServiceName string
}

func (c *ExposeCommand) Info() *cmd.Info {
	return &cmd.Info{"expose", "", "Expose a service.", ""}
}

func (c *ExposeCommand) Init(f *gnuflag.FlagSet, args []string) error {
	addEnvironFlags(&c.EnvName, f)
	if err := f.Parse(true, args); err != nil {
		return err
	}
	args = f.Args()
	switch len(args) {
	case 1:
		c.ServiceName = args[0]
	case 0:
		return errors.New("no service specified")
	default:
		return cmd.CheckEmpty(args[1:])
	}
	return nil
}

// Run exposes a service to the internet.
func (c *ExposeCommand) Run(_ *cmd.Context) error {
	conn, err := juju.NewConn(c.EnvName)
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.Expose(c.ServiceName)
}
