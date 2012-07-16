package environs

import "fmt"

// Get returns the configuration for the respective environment.
// The configuration is validated for the respective provider
// before being returned.
func (e *Environs) Get(name string) (*Config, error) {
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
	return e.config
}

// New returns a new environment based on the provided configuration.
// The configuration is validated for the respective provider before
// the environment is instantiated.
func New(config *config.Config) (Environ, error) {
	p, ok := providers[config.Type()]
	if !ok {
		return nil, fmt.Errorf("no registered provider for %q", kind)
	}
	// TODO Validate config here.
	return p.Open(config)
}
