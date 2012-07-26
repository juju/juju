package juju

import (
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"regexp"
	"sync"
)

var (
	ValidService = regexp.MustCompile("^[a-z][a-z0-9]*(-[a-z0-9]*[a-z][a-z0-9]*)*$")
	ValidUnit    = regexp.MustCompile("^[a-z][a-z0-9]*(-[a-z0-9]*[a-z][a-z0-9]*)*/[0-9]+$")
)

// Conn holds a connection to a juju.
type Conn struct {
	Environ environs.Environ
	state   *state.State
	mu      sync.Mutex
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
	return &Conn{Environ: environ}, nil
}

// NewConnFromAttrs returns a Conn pointing at the environment
// created with the given attributes, as created with environs.NewFromAttrs.
func NewConnFromAttrs(attrs map[string]interface{}) (*Conn, error) {
	environ, err := environs.NewFromAttrs(attrs)
	if err != nil {
		return nil, err
	}
	return &Conn{Environ: environ}, nil
}

// Bootstrap initializes the Conn's environment and makes it ready to deploy
// services.
func (c *Conn) Bootstrap(uploadTools bool) error {
	return c.Environ.Bootstrap(uploadTools)
}

// Destroy destroys the Conn's environment and all its instances.
func (c *Conn) Destroy() error {
	return c.Environ.Destroy(nil)
}

// State returns the environment state associated with c. Closing the
// obtained state will have undefined consequences; Close c instead.
func (c *Conn) State() (*state.State, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state == nil {
		info, err := c.Environ.StateInfo()
		if err != nil {
			return nil, err
		}
		st, err := state.Open(info)
		if err != nil {
			return nil, err
		}
		c.state = st
		if err := c.updateSecrets(); err != nil {
			return nil, err
		}
	}
	return c.state, nil
}

// updateSecrets updates the sensitive parts of the environment 
// from the local configuration.
func (c *Conn) updateSecrets() error {
	cfg := c.Environ.Config()
	env, err := c.state.EnvironConfig()
	if err != nil {
		return err
	}
	secrets, err := c.Environ.Provider().SecretAttrs(cfg)
	if err != nil {
		return err
	}
	env.Update(secrets)
	n, err := env.Write()
	if err != nil {
		return err
	}
	if len(n) > 0 {
		log.Debugf("Updating %d secret(s) in environment %q", len(n), c.Environ.Name())
	}
	return nil
}

// Close terminates the connection to the environment and releases
// any associated resources.
func (c *Conn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	state := c.state
	c.state = nil
	if state != nil {
		return state.Close()
	}
	return nil
}
