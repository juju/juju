package juju

import (
	"fmt"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/trivial"
)

// Conn holds a connection to a juju environment and its
// associated state.
type Conn struct {
	Environ environs.Environ
	State   *state.State
}

// NewConn returns a new Conn that uses the
// given environment. The environment must have already
// been bootstrapped.
func NewConn(environ environs.Environ) (*Conn, error) {
	info, err := environ.StateInfo()
	if err != nil {
		return nil, err
	}
	password := environ.Config().AdminSecret()
	info.Password = password
	st, err := state.Open(info)
	if err == state.ErrUnauthorized {
		// We can't connect with the administrator password,;
		// perhaps this was the first connection and the
		// password has not been changed yet.
		info.Password = trivial.PasswordHash(password)
		st, err = state.Open(info)
		if err != nil {
			return nil, err
		}
		if err := st.SetAdminPassword(password); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	conn := &Conn{
		Environ: environ,
		State:   st,
	}
	if err := conn.updateSecrets(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("unable to push secrets: %v", err)
	}
	return conn, nil
}

// NewConnFromName returns a Conn pointing at the environName environment, or the
// default environment if not specified.
func NewConnFromName(environName string) (*Conn, error) {
	environ, err := environs.NewFromName(environName)
	if err != nil {
		return nil, err
	}
	return NewConn(environ)
}

// Close terminates the connection to the environment and releases
// any associated resources.
func (c *Conn) Close() error {
	return c.State.Close()
}

// updateSecrets writes secrets into the environment when there are none.
// This is done because environments such as ec2 offer no way to securely
// deliver the secrets onto the machine, so the bootstrap is done with the
// whole environment configuration but without secrets, and then secrets
// are delivered on the first communication with the running environment.
func (c *Conn) updateSecrets() error {
	secrets, err := c.Environ.Provider().SecretAttrs(c.Environ.Config())
	if err != nil {
		return err
	}
	cfg, err := c.State.EnvironConfig()
	if err != nil {
		return err
	}
	attrs := cfg.AllAttrs()
	for k := range secrets {
		if _, exists := attrs[k]; exists {
			// Environment already has secrets. Won't send again.
			return nil
		}
	}
	cfg, err = cfg.Apply(secrets)
	if err != nil {
		return err
	}
	return c.State.SetEnvironConfig(cfg)
}
