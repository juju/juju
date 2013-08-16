// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"launchpad.net/loggo"

	"launchpad.net/juju-core/cert"
	"launchpad.net/juju-core/schema"
	"launchpad.net/juju-core/version"
)

var logger = loggo.GetLogger("juju.environs.config")

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

	// DefaultSeries returns the most recent Ubuntu LTS release name.
	DefaultSeries string = "precise"

	// DefaultStatePort is the default port the state server is listening on.
	DefaultStatePort int = 37017

	// DefaultApiPort is the default port the API server is listening on.
	DefaultApiPort int = 17070
)

// Config holds an immutable environment configuration.
type Config struct {
	// m holds the attributes that are defined for Config.
	// t holds the other attributes that are passed in (aka UnknownAttrs).
	// the union of these two are AllAttrs
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
//     ~/.ssh/id_dsa.pub
//     ~/.ssh/id_rsa.pub
//     ~/.ssh/identity.pub
//     ~/.juju/<name>-cert.pem
//     ~/.juju/<name>-private-key.pem
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

	// If the default-series has been explicitly set, but empty, set to the default series.
	if c.asString("default-series") == "" {
		c.m["default-series"] = DefaultSeries
	}

	// Load authorized-keys-path into authorized-keys if necessary.
	path := c.asString("authorized-keys-path")
	keys := c.asString("authorized-keys")
	if path != "" || keys == "" {
		c.m["authorized-keys"], err = readAuthorizedKeys(path)
		if err != nil {
			return nil, err
		}
	}
	delete(c.m, "authorized-keys-path")

	name := c.Name()
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

	// no old config to compare against
	if err = Validate(c, nil); err != nil {
		return nil, err
	}

	// Default firewall mode is instance.
	if c.FirewallMode() == FwDefault {
		c.m["firewall-mode"] = string(FwInstance)
	}

	// Copy unknown attributes onto the type-specific map.
	for k, v := range attrs {
		if _, ok := fields[k]; !ok {
			c.t[k] = v
		}
	}
	return c, nil
}

// Validate ensures that config is a valid configuration.  If old is not nil,
// it holds the previous environment configuration for consideration when
// validating changes.
func Validate(cfg, old *Config) error {

	// Check if there are any required fields that are empty.
	for _, attr := range []string{"name", "type", "default-series", "authorized-keys"} {
		if cfg.asString(attr) == "" {
			return fmt.Errorf("empty %s in environment configuration", attr)
		}
	}

	if strings.ContainsAny(cfg.asString("name"), "/\\") {
		return fmt.Errorf("environment name contains unsafe characters")
	}

	// Check that the agent version parses ok if set explicitly; otherwise leave
	// it alone.
	if v, ok := cfg.m["agent-version"].(string); ok {
		if _, err := version.Parse(v); err != nil {
			return fmt.Errorf("invalid agent version in environment configuration: %q", v)
		}
	}

	// Check firewall mode.
	firewallMode := cfg.FirewallMode()
	switch firewallMode {
	case FwDefault, FwInstance, FwGlobal:
		// Valid mode.
	default:
		return fmt.Errorf("invalid firewall mode in environment configuration: %q", firewallMode)
	}

	// Check the immutable config values.  These can't change
	if old != nil {
		for _, attr := range []string{"type", "name", "firewall-mode"} {
			oldValue := old.asString(attr)
			newValue := cfg.asString(attr)
			if oldValue != newValue {
				return fmt.Errorf("cannot change %s from %q to %q", attr, oldValue, newValue)
			}
		}
		oldStatePort := old.StatePort()
		newStatePort := cfg.StatePort()
		if oldStatePort != newStatePort {
			return fmt.Errorf("cannot change state-port from %d to %d", oldStatePort, newStatePort)
		}
		oldAPIPort := old.APIPort()
		newAPIPort := cfg.APIPort()
		if oldAPIPort != newAPIPort {
			return fmt.Errorf("cannot change api-port from %d to %d", oldAPIPort, newAPIPort)
		}
		if _, oldFound := old.AgentVersion(); oldFound {
			if _, newFound := cfg.AgentVersion(); !newFound {
				return fmt.Errorf("cannot clear agent-version")
			}
		}
	}

	return nil
}

// maybeReadFile sets m[attr] to:
//
// 1) The content of the file m[attr+"-path"], if that's set
// 2) Preserves m[attr] as "" if it was already ""
// 3) The content of defaultPath if it exists and m[attr] is unset
// 4) Preserves the content of m[attr], otherwise
//
// The m[attr+"-path"] key is always deleted.
//
// It returns the data for the attribute, which will be nil
// if the attribute is not set.
func maybeReadFile(m map[string]interface{}, attr, defaultPath string) ([]byte, error) {
	pathAttr := attr + "-path"
	path := m[pathAttr].(string)
	delete(m, pathAttr)
	hasPath := path != ""
	if !hasPath {
		if v, ok := m[attr]; ok {
			if v == "" {
				// The value is explicitly unspecified.
				return nil, nil
			}
			return []byte(v.(string)), nil
		}
		path = defaultPath
	}
	path = expandTilde(path)
	if !filepath.IsAbs(path) {
		path = JujuHomePath(path)
	}
	data, err := ioutil.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) && !hasPath {
			m[attr] = ""
			return nil, nil
		}
		return nil, err
	}
	m[attr] = string(data)
	return data, nil
}

// asString is a private helper method to keep the ugly string casting in once place.
func (c *Config) asString(name string) string {
	value, _ := c.m[name].(string)
	return value
}

// asInt is a private helper method to keep the ugly int casting in once place.
func (c *Config) asInt(name string) int {
	value, _ := c.m[name].(int)
	return value
}

// Type returns the environment type.
func (c *Config) Type() string {
	return c.asString("type")
}

// Name returns the environment name.
func (c *Config) Name() string {
	return c.asString("name")
}

// DefaultSeries returns the default Ubuntu series for the environment.
func (c *Config) DefaultSeries() string {
	return c.asString("default-series")
}

// StatePort returns the state server port for the environment.
func (c *Config) StatePort() int {
	if port := c.asInt("state-port"); port != 0 {
		return port
	}
	return DefaultStatePort
}

// APIPort returns the API server port for the environment.
func (c *Config) APIPort() int {
	if port := c.asInt("api-port"); port != 0 {
		return port
	}
	return DefaultApiPort
}

// AuthorizedKeys returns the content for ssh's authorized_keys file.
func (c *Config) AuthorizedKeys() string {
	return c.asString("authorized-keys")
}

// CACert returns the certificate of the CA that signed the state server
// certificate, in PEM format, and whether the setting is available.
func (c *Config) CACert() ([]byte, bool) {
	if s := c.asString("ca-cert"); s != "" {
		return []byte(s), true
	}
	return nil, false
}

// CAPrivateKey returns the private key of the CA that signed the state
// server certificate, in PEM format, and whether the setting is available.
func (c *Config) CAPrivateKey() (key []byte, ok bool) {
	if s := c.asString("ca-private-key"); s != "" {
		return []byte(s), true
	}
	return nil, false
}

// AdminSecret returns the administrator password.
// It's empty if the password has not been set.
func (c *Config) AdminSecret() string {
	return c.asString("admin-secret")
}

// FirewallMode returns whether the firewall should
// manage ports per machine or global.
func (c *Config) FirewallMode() FirewallMode {
	return FirewallMode(c.asString("firewall-mode"))
}

// AgentVersion returns the proposed version number for the agent tools,
// and whether it has been set. Once an environment is bootstrapped, this
// must always be valid.
func (c *Config) AgentVersion() (version.Number, bool) {
	if v, ok := c.m["agent-version"].(string); ok {
		n, err := version.Parse(v)
		if err != nil {
			panic(err) // We should have checked it earlier.
		}
		return n, true
	}
	return version.Zero, false
}

// ToolsMetadataURL returns the URL at which the metadata used to
// locate tools tarballs is located.
func (c *Config) ToolsMetadataURL() string {
	return c.asString("tools-metadata-url")
}

// ImageMetadataURL returns the URL at which the metadata used to
// locate image ids is located.
func (c *Config) ImageMetadataURL() string {
	return c.asString("image-metadata-url")
}

// Development returns whether the environment is in development mode.
func (c *Config) Development() bool {
	return c.m["development"].(bool)
}

// SSLHostnameVerification returns weather the environment has requested
// SSL hostname verification to be enabled.
func (c *Config) SSLHostnameVerification() bool {
	return c.m["ssl-hostname-verification"].(bool)
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
	"type":                      schema.String(),
	"name":                      schema.String(),
	"default-series":            schema.String(),
	"tools-metadata-url":        schema.String(),
	"image-metadata-url":        schema.String(),
	"authorized-keys":           schema.String(),
	"authorized-keys-path":      schema.String(),
	"firewall-mode":             schema.String(),
	"agent-version":             schema.String(),
	"development":               schema.Bool(),
	"admin-secret":              schema.String(),
	"ca-cert":                   schema.String(),
	"ca-cert-path":              schema.String(),
	"ca-private-key":            schema.String(),
	"ca-private-key-path":       schema.String(),
	"ssl-hostname-verification": schema.Bool(),
	"state-port":                schema.ForceInt(),
	"api-port":                  schema.ForceInt(),
}

var defaults = schema.Defaults{
	"default-series":            DefaultSeries,
	"tools-metadata-url":        "",
	"image-metadata-url":        "",
	"authorized-keys":           "",
	"authorized-keys-path":      "",
	"firewall-mode":             FwDefault,
	"agent-version":             schema.Omit,
	"development":               false,
	"admin-secret":              "",
	"ca-cert":                   schema.Omit,
	"ca-cert-path":              "",
	"ca-private-key":            schema.Omit,
	"ca-private-key-path":       "",
	"ssl-hostname-verification": true,
	"state-port":                schema.Omit,
	"api-port":                  schema.Omit,
}

var checker = schema.FieldMap(fields, defaults)

// ValidateUnknownAttrs checks the unknown attributes of the config against
// the supplied fields and defaults, and returns an error if any fails to
// validate. Unknown fields are warned about, but preserved, on the basis
// that they are reasonably likely to have been written by or for a version
// of juju that does recognise the fields, but that their presence is still
// anomalous to some degree and should be flagged (and that there is thereby
// a mechanism for observing fields that really are typos etc).
func (cfg *Config) ValidateUnknownAttrs(fields schema.Fields, defaults schema.Defaults) (map[string]interface{}, error) {
	attrs := cfg.UnknownAttrs()
	checker := schema.FieldMap(fields, defaults)
	coerced, err := checker.Coerce(attrs, nil)
	if err != nil {
		return nil, err
	}
	result := coerced.(map[string]interface{})
	for name, value := range attrs {
		if fields[name] == nil {
			logger.Warningf("unknown config field %q", name)
			result[name] = value
		}
	}
	return result, nil
}

// GenerateStateServerCertAndKey makes sure that the config has a CACert and
// CAPrivateKey, generates and retruns new certificate and key.
func (cfg *Config) GenerateStateServerCertAndKey() ([]byte, []byte, error) {
	caCert, hasCACert := cfg.CACert()
	if !hasCACert {
		return nil, nil, fmt.Errorf("environment configuration has no ca-cert")
	}
	caKey, hasCAKey := cfg.CAPrivateKey()
	if !hasCAKey {
		return nil, nil, fmt.Errorf("environment configuration has no ca-private-key")
	}
	return cert.NewServer(cfg.Name(), caCert, caKey, time.Now().UTC().AddDate(10, 0, 0))
}
