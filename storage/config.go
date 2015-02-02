// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

// Config defines the configuration for a storage source.
type Config struct {
	name     string
	provider ProviderType
	attrs    map[string]interface{}
}

// NewConfig creates a new Config for instantiating a storage source.
func NewConfig(name string, provider ProviderType, attrs map[string]interface{}) (*Config, error) {
	// TODO(axw) validate attributes.
	return &Config{name, provider, attrs}, nil
}

// Name returns the name of a storage source. This is not necessarily unique,
// and should only be used for informational purposes.
func (c *Config) Name() string {
	return c.name
}

// Provider returns the name of a storage provider.
func (c *Config) Provider() ProviderType {
	return c.provider
}

// Attrs returns the configuration attributes for a storage source.
func (c *Config) Attrs() map[string]interface{} {
	attrs := make(map[string]interface{})
	for k, v := range c.attrs {
		attrs[k] = v
	}
	return attrs
}

// ValueString returns the named config attribute as a string.
func (c *Config) ValueString(name string) (string, bool) {
	v, ok := c.attrs[name].(string)
	return v, ok
}
