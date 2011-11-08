package juju

import "fmt"

// New creates a new Environ using the
// environment configuration with the given name.
// If name is empty, the default environment will be used.
func (envs *Environs) Open(name string) (Environ, error) {
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
	env, err := providers[e.kind].Open(name, e.config)
	if err != nil {
		return nil, fmt.Errorf("cannot initialize environment %q: %v", name, err)
	}
	return env, nil
}
