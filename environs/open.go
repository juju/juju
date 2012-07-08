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

// NewConfig validates and returns a provider specific EnvironConfig using the
// configuration attributes supplied, which should include the environment name.
func NewConfig(attrs map[string]interface{}) (EnvironConfig, error) {
	kind, ok := attrs["type"].(string)
	if !ok {
		return nil, fmt.Errorf("no provider type given")
	}
	p, ok := providers[kind]
	if !ok {
		return nil, fmt.Errorf("no registered provider for %q", kind)
	}
	return p.NewConfig(attrs)
}

// NewEnviron creates a new Environ of the registered type using the configuration
// attributes supplied, which should include the environment name.
func NewEnviron(attrs map[string]interface{}) (Environ, error) {
	cfg, err := NewConfig(attrs)
	if err != nil {
		return nil, fmt.Errorf("error validating environment: %v", err)
	}
	return cfg.Open()
}
