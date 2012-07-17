package config

import (
	"fmt"
	"launchpad.net/juju-core/schema"
)


// Config holds an immutable environment configuration.
type Config struct {
	m, t map[string]interface{}
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
	c := &Config{
		m: make(map[string]interface{}),
		t: make(map[string]interface{}),
	}
	for k, v := range m.(schema.MapType) {
		c.m[k.(string)] = v
	}
	if _, ok := c.m["default-series"]; !ok {
		c.m["default-series"] = CurrentSeries
	}

	// Load authorized-keys-path onto authorized-keys, if necessary.
	path, _ := c.m["authorized-keys-path"].(string)
	keys, _ := c.m["authorized-keys"].(string)
	if path != "" || keys == "" {
		c.m["authorized-keys"], err = authorizedKeys(path)
		if err != nil {
			return nil, err
		}
		delete(c.m, "authorized-keys-path")
	}

	// Check if there are any required fields that are empty.
	for _, attr := range []string{"name", "type", "authorized-keys"} {
		if s, _ := c.m[attr].(string); s == "" {
			return nil, fmt.Errorf("empty %s in environment configuration", attr)
		}
	}

	// Copy unknown attributes onto the type-specific map.
	for k, v := range attrs {
		if _, ok := fields[k]; !ok {
			c.t[k] = v
		}
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

// AuthorizedKeys returns the content for ssh's authorized_keys file.
func (c *Config) AuthorizedKeys() string {
	return c.m["authorized-keys"].(string)
}

// TypeMap returns a copy of the raw configuration attributes that are
// specific to the environment type.
func (c *Config) TypeMap() map[string]interface{} {
	t := make(map[string]interface{})
	for k, v := range c.t {
		t[k] = v
	}
	return t
}

// Map returns a copy of the raw configuration attributes.
func (c *Config) Map() map[string]interface{} {
	m := c.TypeMap()
	for k, v := range c.m {
		m[k] = v
	}
	return m
}

var fields = schema.Fields{
	"type":                 schema.String(),
	"name":                 schema.String(),
	"default-series":       schema.String(),
	"authorized-keys":      schema.String(),
	"authorized-keys-path": schema.String(),
}

var checker = schema.FieldMap(
	fields,
	[]string{
		"default-series",
		"authorized-keys",
		"authorized-keys-path",
	},
)

