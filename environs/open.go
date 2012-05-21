package environs

import "fmt"

// Open creates a new Environ using the environment configuration with the 
// given name. If name is empty, the default environment will be used.
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
	return e.config.Open()
}

// NewEnviron creates a new Environ of the registered kind using the configuration
// attributes supplied, which should include the environment name.
func NewEnviron(kind string, attrs map[string]interface{}) (Environ, error) {
	p, ok := providers[kind]
	if !ok {
		return nil, fmt.Errorf("no registered provider for kind: %q", kind)
	}
	cfg, err := p.NewConfig(attrs)
	if err != nil {
		return nil, fmt.Errorf("error validating environment: %v", err)
	}
	return cfg.Open()
}
