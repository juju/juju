package config

import (
	"fmt"
	"launchpad.net/juju-core/schema"
	"launchpad.net/juju-core/version"
	"os"
)

// FirewallMode defines the way in which the environment
// handles opening and closing of firewall ports.
type FirewallMode string

const (
	// FwDefault is the environment-specific default mode.
	FwDefault FirewallMode = ""

	// FwInstance requests the use of an individual firewall per instance.
	FwInstance FirewallMode = "instance"

	// FwGlobal requests the use of a single firewall group for all machines.
	// When ports are opened for one machine, all machines will have the same
	// port opened.
	FwGlobal FirewallMode = "global"
)

// Config holds an immutable environment configuration.
type Config struct {
	m, t map[string]interface{}
}

// New returns a new configuration.
// Fields that are common to all environment providers are verified.
// The "authorized-keys-path" key is translated into "authorized-keys"
// by loading the content from respective file; similarly,
// "root-cert-path" is translated into the "root-cert" and "root-private-key"
// keys.
//
// The required keys are: "name", "type" and "authorized-keys",
// all of type string. Additional keys recognised are: "agent-version" and
// "development", of types string and bool respectively.
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

	// Load authorized-keys-path into authorized-keys if necessary.
	path := c.m["authorized-keys-path"].(string)
	keys := c.m["authorized-keys"].(string)
	if path != "" || keys == "" {
		c.m["authorized-keys"], err = readAuthorizedKeys(path)
		if err != nil {
			return nil, err
		}
	}
	delete(c.m, "authorized-keys-path")

	// Load root certificate into root-cert if necessary.
	rootCert := []byte(c.m["root-cert"].(string))
	rootCertPath := c.m["root-cert-path"].(string)
	if rootCertPath != "" || len(rootCert) == 0 {
		rootCert, err = readCertFile(rootCertPath, "rootcert.pem")
		if err != nil {
			return nil, err
		}
		c.m["root-cert"] = string(rootCert)
	}
	delete(c.m, "root-cert-path")

	// Load root key into root-private-cert if necessary.
	var rootKey []byte
	if k, ok := c.m["root-private-key"]; ok {
		rootKey = []byte(k.(string))
	}
	rootKeyPath := c.m["root-private-key-path"].(string)
	if rootKeyPath != "" || rootKey == nil {
		rootKey, err = readCertFile(rootKeyPath, "rootkey.pem")
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		c.m["root-private-key"] = string(rootKey)
	}
	delete(c.m, "root-private-key-path")

	if err := verifyKeyPair(rootCert, rootKey); err != nil {
		if rootCertPath == "" {
			return nil, fmt.Errorf("bad root certificate/key in configuration: %v", err)
		}
		return nil, fmt.Errorf("bad root certificate in %q: %v", rootCertPath, err)
	}

	// Check if there are any required fields that are empty.
	for _, attr := range []string{"name", "type", "default-series", "authorized-keys", "root-cert"} {
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

	// Check firewall mode.
	firewallMode := FirewallMode(c.m["firewall-mode"].(string))
	switch firewallMode {
	case FwDefault, FwInstance, FwGlobal:
		// Valid mode.
	default:
		return nil, fmt.Errorf("invalid firewall mode in environment configuration: %q", firewallMode)
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

// RootCertPEM returns the X509 certificate for the
// root certifying authority, in PEM format.
func (c *Config) RootCertPEM() string {
	return c.m["root-cert"].(string)
}

// RootPrivateKeyPEM returns the private key of the
// root certifying authority, in PEM format.
// It is empty if the key is not available.
func (c *Config) RootPrivateKeyPEM() string {
	return c.m["root-private-key"].(string)
}

// AdminSecret returns the administrator password.
// It's empty if the password has not been set.
func (c *Config) AdminSecret() string {
	return c.m["admin-secret"].(string)
}

// FirewallMode returns whether the firewall should
// manage ports per machine or global.
func (c *Config) FirewallMode() FirewallMode {
	return FirewallMode(c.m["firewall-mode"].(string))
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

// Development returns whether the environment is in development
// mode.
func (c *Config) Development() bool {
	return c.m["development"].(bool)
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
	"type":                  schema.String(),
	"name":                  schema.String(),
	"default-series":        schema.String(),
	"authorized-keys":       schema.String(),
	"authorized-keys-path":  schema.String(),
	"firewall-mode":         schema.String(),
	"agent-version":         schema.String(),
	"development":           schema.Bool(),
	"admin-secret":          schema.String(),
	"root-cert":             schema.String(),
	"root-cert-path":        schema.String(),
	"root-private-key":      schema.String(),
	"root-private-key-path": schema.String(),
}

var defaults = schema.Defaults{
	"default-series":        version.Current.Series,
	"authorized-keys":       "",
	"authorized-keys-path":  "",
	"firewall-mode":         FwDefault,
	"agent-version":         schema.Omit,
	"development":           false,
	"admin-secret":          "",
	"root-cert-path":        "",
	"root-cert":             "",
	"root-private-key-path": "",
	"root-private-key":      schema.Omit,
}

var checker = schema.FieldMap(fields, defaults)
