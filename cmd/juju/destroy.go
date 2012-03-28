package main

import (
	"launchpad.net/juju/go/cmd"
	"launchpad.net/juju/go/juju"
	"log"
	"launchpad.net/gnuflag"
)

// DestroyCommand destroys an environment.
type DestroyCommand struct {
	envName string
	Conn *juju.Conn
}

func (c *DestroyCommand) Info() *cmd.Info {
	return &cmd.Info{
		"destroy-environment", "[options]",
		"terminate all machines and other associated resources for an environment",
		"",
		true,
	}
}

func (c *DestroyCommand) InitFlagSet(f *gnuflag.FlagSet) {
	f.StringVar(&c.envName, "e", "", "juju environment to operate in")
	f.StringVar(&c.envName, "environment", "", "")
}

func (c *DestroyCommand) ParsePositional(args []string) (err error) {
	c.Conn, err = juju.NewConn(c.envName)
	if err != nil {
		return err
	}
	return cmd.CheckEmpty(args)
}

func (c *DestroyCommand) Run() error {
log.Printf("calling conn.Destroy on %v\n", c.Conn)
	return c.Conn.Destroy()
}
