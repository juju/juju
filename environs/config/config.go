package config

import (
	"fmt"
	"launchpad.net/juju-core/schema"
	"launchpad.net/juju-core/version"
)

// Config holds an immutable environment configuration.
type Config struct {
	m, t map[string]interface{}
}

// New returns a new configuration.
// Fields that are common to all environment providers are verified,
// and authorized-keys-path is also translated into authorized-keys
// by loading the content from respective file.
//
// The required keys are: "name", "type", "default-series" and "authorized-keys",
// all of type string. Additional keys recognised are: "agent-version" and
// "dev-version", of types string and bool respectively.
func New(attrs map[string]interface{}) (*Config, error) {
	m, err := checker.Coerce(attrs, nil)
	if err != nil {
		return nil, err
	}
	c := &Config{
		m: m.(map[string]interface{}),
		t: make(map[string]interface{}),
	}

	if c.m["default-series"].(string) == "" {
		c.m["default-series"] = version.Current.Series
	}

	// Load authorized-keys-path onto authorized-keys, if necessary.
	path := c.m["authorized-keys-path"].(string)
	keys := c.m["authorized-keys"].(string)
	if path != "" || keys == "" {
		c.m["authorized-keys"], err = authorizedKeys(path)
		if err != nil {
			return nil, err
		}
	}
	delete(c.m, "authorized-keys-path")

	// Check if there are any required fields that are empty.
	for _, attr := range []string{"name", "type", "default-series", "authorized-keys"} {
		if s, _ := c.m[attr].(string); s == "" {
			return nil, fmt.Errorf("empty %s in environment configuration", attr)
		}
	}

	// Check that the agent version parses ok if set.
	if v, ok := c.m["agent-version"].(string); ok {
		if _, err := version.Parse(v); err != nil {
			return nil, fmt.Errorf("invalid agent version in environment configuration: %q", v)
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

// Type returns the environment type.
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

// AgentVersion returns the proposed version number for the agent tools.
// It returns the zero version if unset.
func (c *Config) AgentVersion() version.Number {
	v, ok := c.m["agent-version"].(string)
	if !ok {
		return version.Number{}
	}
	n, err := version.Parse(v)
	if err != nil {
		panic(err) // We should have checked it earlier.
	}
	return n
}

// DevVersion returns whether upgrades are allowed from release versions
// to development versions.
func (c *Config) DevVersion() bool {
	v, _ := c.m["dev-version"].(bool)
	return v
}

// UnknownAttrs returns a copy of the raw configuration attributes
// that are supposedly specific to the environment type. They could
// also be wrong attributes, though. Only the specific environment
// implementation can tell.
func (c *Config) UnknownAttrs() map[string]interface{} {
	t := make(map[string]interface{})
	for k, v := range c.t {
		t[k] = v
	}
	return t
}

// AllAttrs returns a copy of the raw configuration attributes.
func (c *Config) AllAttrs() map[string]interface{} {
	m := c.UnknownAttrs()
	for k, v := range c.m {
		m[k] = v
	}
	return m
}

// Apply returns a new configuration that has the attributes of c plus attrs.
func (c *Config) Apply(attrs map[string]interface{}) (*Config, error) {
	m := c.AllAttrs()
	for k, v := range attrs {
		m[k] = v
	}
	return New(m)
}

var fields = schema.Fields{
	"type":                 schema.String(),
	"name":                 schema.String(),
	"default-series":       schema.String(),
	"authorized-keys":      schema.String(),
	"authorized-keys-path": schema.String(),
	"agent-version":        schema.String(),
	"dev-version":          schema.Bool(),
}

var defaults = schema.Defaults{
	"default-series":       version.Current.Series,
	"authorized-keys":      "",
	"authorized-keys-path": "",
	"agent-version":        schema.Omit,
	"dev-version":          schema.Omit,
}

var checker = schema.FieldMap(fields, defaults)
