package juju

import (
	"fmt"
	"os"
)

// Conn represents a connection to a juju environment.
type Conn struct {
	env Environ // the instantiated environment.
}

// New returns a new Conn using the named environment.
// If name is empty, the default environment will be used.
func (envs *Environs) New(name string) (*Conn, os.Error) {
	if name == "" {
		name = envs.Default
		if name == "" {
			return nil, fmt.Errorf("no default environment found")
		}
	}
	e, ok := envs.environs[name]
	if !ok {
		return nil, fmt.Errorf("unknown environment %q", name)
	}
	if e.err != nil {
		return nil, e.err
	}
	penv, err := providers[e.kind].NewEnviron(name, e.config)
	if err != nil {
		return nil, fmt.Errorf("cannot initialize environment %q: %v", name, err)
	}
	return &Conn{
		env: penv,
	}, nil
}

// Environ returns the environment used by the Conn.
// It can be used to subvert juju methods, so care is
// required in using it.
func (c *Conn) Environ() Environ {
	return c.env
}

// Bootstrap bootstraps a juju environment for the first time.
func (c *Conn) Bootstrap() os.Error {
	if err := c.env.Bootstrap(); err != nil {
		return fmt.Errorf("cannot bootstrap environment: %v", err)
	}
	return nil
}

// Destroy destroys a juju environment and all resources associated
// with it.
func (c *Conn) Destroy() os.Error {
	return c.env.Destroy()
}
