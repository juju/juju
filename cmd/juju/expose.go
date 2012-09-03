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
	return &cmd.Info{"expose", "", "expose a service", ""}
}

func (c *ExposeCommand) Init(f *gnuflag.FlagSet, args []string) error {
	addEnvironFlags(&c.EnvName, f)
	if err := f.Parse(true, args); err != nil {
		return err
	}
	args = f.Args()
	if len(args) == 0 {
		return errors.New("no service name specified")
	}
	c.ServiceName = args[0]
	return cmd.CheckEmpty(args[1:])
}

// Run changes the juju-managed firewall to expose any
// ports that were also explicitly marked by units as open.
func (c *ExposeCommand) Run(_ *cmd.Context) error {
	conn, err := juju.NewConn(c.EnvName)
	if err != nil {
		return err
	}
	defer conn.Close()
	svc, err := conn.State.Service(c.ServiceName)
	if err != nil {
		return err
	}
	return svc.SetExposed()
}
