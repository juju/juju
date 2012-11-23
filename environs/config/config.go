package config

import (
	"fmt"
	"io/ioutil"
	"launchpad.net/juju-core/schema"
	"launchpad.net/juju-core/version"
	"os"
	"path/filepath"
	"strings"
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

// TODO(rog) update the doc comment below - it's getting messy
// and it assumes too much prior knowledge.

// New returns a new configuration.  Fields that are common to all
// environment providers are verified.  The "authorized-keys-path" key
// is translated into "authorized-keys" by loading the content from
// respective file.  Similarly, "ca-cert-path" and "ca-private-key-path"
// are translated into the "ca-cert" and "ca-private-key" values.  If
// not specified, authorized SSH keys and CA details will be read from:
//
//	~/.ssh/id_dsa.pub
//	~/.ssh/id_rsa.pub
//	~/.ssh/identity.pub
//	~/.juju/<name>-cert.pem
//	~/.juju/<name>-private-key.pem
//
// The ca-cert and ca-private-key attributes may be explicitly
// provided as nil values.
//
// The required keys (after any files have been read) are "name",
// "type" and "authorized-keys", all of type string.  Additional keys
// recognised are "agent-version" and "development", of types string
// and bool respectively.
func New(attrs map[string]interface{}) (*Config, error) {
	m, err := checker.Coerce(attrs, nil)
	if err != nil {
		return nil, err
	}
	c := &Config{
		m: m.(map[string]interface{}),
		t: make(map[string]interface{}),
	}

	name := c.m["name"].(string)
	if name == "" {
		return nil, fmt.Errorf("empty name in environment configuration")
	}
	if strings.ContainsAny(name, "/\\") {
		return nil, fmt.Errorf("environment name contains unsafe characters")
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

	caCert, err := maybeReadFile(c.m, "ca-cert", name+"-cert.pem")
	if err != nil {
		return nil, err
	}
	caKey, err := maybeReadFile(c.m, "ca-private-key", name+"-private-key.pem")
	if err != nil {
		return nil, err
	}
	if caCert != nil || caKey != nil {
		if err := verifyKeyPair(caCert, caKey); err != nil {
			return nil, fmt.Errorf("bad CA certificate/key in configuration: %v", err)
		}
	}

	// Check if there are any required fields that are empty.
	for _, attr := range []string{"type", "default-series", "authorized-keys"} {
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

// maybeReadFile reads the given attribute from a file if necessary,
// sets the attribute in attrs and deletes the associated path
// attribute.  It returns the data for the attribute, which will be nil
// if the attribute is not set.
func maybeReadFile(attrs map[string]interface{}, attr, defaultPath string) ([]byte, error) {
	pathAttr := attr + "-path"
	path := attrs[pathAttr].(string)
	delete(attrs, pathAttr)
	if path == "" {
		if v, ok := attrs[attr]; ok {
			if v == nil {
				// The value is explicitly unspecified.
				return nil, nil
			}
			if s := v.(string); s != "" {
				// "" means default.
				return []byte(s), nil
			}
		}
		path = defaultPath
	}
	path = expandTilde(path)
	if !filepath.IsAbs(path) {
		path = filepath.Join(os.Getenv("HOME"), ".juju", path)
	}
	data, err := ioutil.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			attrs[attr] = nil
			return nil, nil
		}
		return nil, err
	}
	attrs[attr] = string(data)
	return data, nil
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

// CACertPEM returns the X509 certificate for the
// certifying authority, in PEM format.
// It returns false if the certificate is not present.
func (c *Config) CACertPEM() ([]byte, bool) {
	s, ok := c.m["ca-cert"].(string)
	return []byte(s), ok
}

// CAPrivateKeyPEM returns the private key of the
// certifying authority, in PEM format.
// It returns false if the key is not present.
func (c *Config) CAPrivateKeyPEM() (key []byte, ok bool) {
	s, ok := c.m["ca-private-key"].(string)
	return []byte(s), ok
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
	"type":                 schema.String(),
	"name":                 schema.String(),
	"default-series":       schema.String(),
	"authorized-keys":      schema.String(),
	"authorized-keys-path": schema.String(),
	"firewall-mode":        schema.String(),
	"agent-version":        schema.String(),
	"development":          schema.Bool(),
	"admin-secret":         schema.String(),
	"ca-cert":              schema.OneOf(schema.String(), schema.Const(nil)),
	"ca-cert-path":         schema.String(),
	"ca-private-key":       schema.OneOf(schema.String(), schema.Const(nil)),
	"ca-private-key-path":  schema.String(),
}

var defaults = schema.Defaults{
	"default-series":       version.Current.Series,
	"authorized-keys":      "",
	"authorized-keys-path": "",
	"firewall-mode":        FwDefault,
	"agent-version":        schema.Omit,
	"development":          false,
	"admin-secret":         "",
	"ca-cert":              "",
	"ca-cert-path":         "",
	"ca-private-key":       "",
	"ca-private-key-path":  "",
}

var checker = schema.FieldMap(fields, defaults)
