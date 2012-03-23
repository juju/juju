package main
import (
	"fmt"
	"launchpad.net/juju/go/juju"
	"launchpad.net/gnuflag"
)

// conn holds a juju connection and implements
// cmd.Command.InitFlagSet to define the -e and --environment
// flags.
type conn struct {
	Conn *juju.Conn
}

// connFlag is used instead of conn so that types
// embedding conn don't gain an inappropriate
// Set method.
type connFlag conn

func (c *conn) InitFlagSet(f *gnuflag.FlagSet) {
	f.StringVar((*connFlag)(c), "e", "", "juju environment to operate in")
	f.StringVar((*connFlag)(c), "environment", "", "")
}

func (c *connFlag) Set(value string) (err error) {
	c.Conn, err = juju.NewConn(value)
	if err != nil {
		err = fmt.Errorf("error opening environment: %v", err)
	}
	return
}
