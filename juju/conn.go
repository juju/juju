package juju

import (
	"fmt"
	"launchpad.net/juju/go/environs"
)

// Conn holds a connection to a juju.
type Conn struct {
	// TODO extend to hold an optional Zookeeper connection as well.
	environ environs.Environ
	state   *state.State
}

// NewConn returns a Conn pointing at the environName environment, or the
// default environment if not specified.
func NewConn(environName string) (*Conn, error) {
	environs, err := environs.ReadEnvirons("")
	if err != nil {
		return nil, err
	}
	environ, err := environs.Open(environName)
	if err != nil {
		return nil, err
	}
	return &Conn{environ}, nil
}

func (c *Conn) Connect() error {
	state, err := c.environ.StateInfo().Connect()
	if err != nil {
		return err
	}
	c.state = state
	return nil
}

// Bootstrap initializes the Conn's environment and makes it ready to deploy
// services.
func (c *Conn) Bootstrap() error {
	return c.environ.Bootstrap()
}
