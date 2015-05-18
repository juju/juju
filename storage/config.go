// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/errors"
	"github.com/juju/schema"
)

const (
	// ConfigStorageDir is the path to the directory which a
	// machine-scoped storage source may use to contain storage
	// artifacts. This should not be used for environment-wide
	// storage sources, as the contents are bound to the
	// lifetime of the machine.
	//
	// ConfigStorageDir is set by the storage provisioner, so
	// should not be relied upon until a storage source is
	// constructed.
	ConfigStorageDir = "storage-dir"

	// Persistent is true if storage survives the lifecycle of the
	// machine to which it is attached.
	Persistent = "persistent"
)

// Config defines the configuration for a storage source.
type Config struct {
	name       string
	provider   ProviderType
	attrs      map[string]interface{}
	persistent bool
}

var fields = schema.Fields{
	Persistent: schema.Bool(),
}

var configChecker = schema.FieldMap(
	fields,
	schema.Defaults{
		Persistent: false,
	},
)

// NewConfig creates a new Config for instantiating a storage source.
func NewConfig(name string, provider ProviderType, attrs map[string]interface{}) (*Config, error) {
	out, err := configChecker.Coerce(attrs, nil)
	if err != nil {
		return nil, errors.Annotate(err, "validating common storage config")
	}
	coerced := out.(map[string]interface{})
	return &Config{
		name:       name,
		provider:   provider,
		attrs:      attrs,
		persistent: coerced[Persistent].(bool),
	}, nil
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
	if c.attrs == nil {
		return nil
	}
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

// IsPersistent returns true if config has persistent set to true.
func (c *Config) IsPersistent() bool {
	return c.persistent
}
