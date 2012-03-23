package juju

import (
	"launchpad.net/juju/go/environs"
	"regexp"
)

var (
	ValidService = regexp.MustCompile("^[a-z][a-z0-9]*(-[a-z0-9]*[a-z][a-z0-9]*)*$")
	ValidUnit    = regexp.MustCompile("^[a-z][a-z0-9]*(-[a-z0-9]*[a-z][a-z0-9]*)*/[0-9]+$")
)

// Conn holds a connection to a juju.
type Conn struct {
	// TODO extend to hold a *state.State.
	Environ environs.Environ
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

// Bootstrap initializes the Conn's environment and makes it ready to deploy
// services.
func (c *Conn) Bootstrap() error {
	return c.Environ.Bootstrap()
}

// Destroy destroys the Conn's environment and all its instances.
func (c *Conn) Destroy() error {
	return c.Environ.Destroy()
}
