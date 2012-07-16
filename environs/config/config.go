package config

import (
	"launchpad.net/juju-core/schema"
)


// Config holds an immutable environment configuration.
type Config struct {
	m map[string]interface{}
}

// New returns a new configuration.
// Fields that are common to all environment providers are verified,
// and authorized-keys-path is also translated into authorized-keys
// by loading the content from respective file.
func New(attrs map[string]interface{}) (*Config, error) {
	m, err := checker.Coerce(attrs, nil)
	if err != nil {
		return nil, err
	}
	c := &Config{make(map[string]interface{})}
	for k, v := range m.(schema.MapType) {
		c.m[k.(string)] = v
	}
	if _, ok := c.m["default-series"]; !ok {
		c.m["default-series"] = CurrentSeries
	}
	return c, nil
}

// Type returns the enviornment type.
func (c *Config) Type() string {
	return c.m["type"].(string)
}

// Name returns the environment name.
func (c *Config) Name() string {
	return c.m["name"].(string)
}

// DefaultSeries returns the default Ubuntu series for the environment.
func (c *Config) DefaultSeries() string {
	return c.m["default-series"].(string)
}

// Map returns a copy of the raw configuration attributes.
func (c *Config) Map() map[string]interface{} {
	m := make(map[string]interface{})
	for k, v := range c.m {
		m[k] = v
	}
	return m
}

// TypeMap returns a copy of the raw configuration attributes that are
// specific to the environment type.
func (c *Config) TypeMap() map[string]interface{} {
	m := make(map[string]interface{})
	for k, v := range c.m {
		if _, ok := fields[k]; !ok {
			m[k] = v
		}
	}
	return m
}

var fields = schema.Fields{
	"type":                 schema.String(),
	"name":                 schema.String(),
	"default-series":       schema.String(),
	// TODO Move authorized-keys and authorized-keys-path from ec2.
}

var checker = schema.FieldMap(
	fields,
	[]string{
		"default-series",
	},
)

