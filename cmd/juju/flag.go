package main

import (
	"launchpad.net/gnuflag"
	"launchpad.net/juju/go/juju"
)

// conn holds a juju connection and implements
// cmd.Command.InitFlagSet to define the -e and --environment
// flags.
type conn struct {
	envName string
	Conn    *juju.Conn
}

// InitFlagSet defines the -e and -environment flags.
func (c *conn) InitFlagSet(f *gnuflag.FlagSet) {
	f.StringVar(&c.envName, "e", "", "juju environment to operate in")
	f.StringVar(&c.envName, "environment", "", "")
}

// InitConn opens the environment and sets c.Conn to it.
func (c *conn) InitConn() (err error) {
	c.Conn, err = juju.NewConn(c.envName)
	return
}
