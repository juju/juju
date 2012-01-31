package juju

import (
	"fmt"
	"launchpad.net/juju/go/environs"
)

// Conn currently just holds an Environ, but will at some stage be extended to
// hold a Zookeeper connection (when appropriate) as well.
type Conn struct {
	environ environs.Environ
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

// Bootstrap should initialize the Conn's environment and make it ready to
// deploy services.
func (c *Conn) Bootstrap() error {
	return fmt.Errorf("This doesn't do anything yet.")
}
