package juju

import (
	"fmt"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/state"
	"regexp"
)

var (
	ValidService = regexp.MustCompile("^[a-z][a-z0-9]*(-[a-z0-9]*[a-z][a-z0-9]*)*$")
	ValidUnit    = regexp.MustCompile("^[a-z][a-z0-9]*(-[a-z0-9]*[a-z][a-z0-9]*)*/[0-9]+$")
)

// Conn holds a connection to a juju environment and its
// associated state.
type Conn struct {
	Environ environs.Environ
	State   *state.State
}

// NewConnFromEnviron returns a new Conn that uses the
// given environment. The environment must have already
// been bootstrapped.
func NewConnFromEnviron(environ environs.Environ) (*Conn, error) {
	info, err := environ.StateInfo()
	if err != nil {
		return nil, err
	}
	st, err := state.Open(info)
	if err != nil {
		return nil, err
	}
	// Update secrets in the environment.
	// This is wrong. This will _always_ overwrite the secrets
	// in the state with the local secrets. To fix this properly
	// we need to ensure that the config, minus secrets, is always
	// pushed on bootstrap, then we can fill in the secrets here.
	if err := st.SetEnvironConfig(environ.Config()); err != nil {
		st.Close()
		return nil, fmt.Errorf("unable to push secrets: %v", err)
	}
	return &Conn{
		Environ: environ,
		State:   st,
	}, nil
}

// NewConn returns a Conn pointing at the environName environment, or the
// default environment if not specified.
func NewConn(environName string) (*Conn, error) {
	environ, err := environs.NewFromName(environName)
	if err != nil {
		return nil, err
	}
	return NewConnFromEnviron(environ)
}

// NewConnFromAttrs returns a Conn pointing at the environment
// created with the given attributes, as created with environs.NewFromAttrs.
func NewConnFromAttrs(attrs map[string]interface{}) (*Conn, error) {
	environ, err := environs.NewFromAttrs(attrs)
	if err != nil {
		return nil, err
	}
	return NewConnFromEnviron(environ)
}

// Close terminates the connection to the environment and releases
// any associated resources.
func (c *Conn) Close() error {
	if c == nil {
		panic("nil conn")
	}
	if c.State == nil {
		panic("nil state")
	}
	return c.State.Close()
}
