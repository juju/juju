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

const (
	// FwInstance requests the use of an individual firewall per instance.
	FwInstance = "instance"

	// FwGlobal requests the use of a single firewall group for all machines.
	// When ports are opened for one machine, all machines will have the same
	// port opened.
	FwGlobal = "global"

	// DefaultSeries returns the most recent Ubuntu LTS release name.
	DefaultSeries string = "precise"

	// DefaultStatePort is the default port the state server is listening on.
	DefaultStatePort int = 37017

	// DefaultApiPort is the default port the API server is listening on.
	DefaultAPIPort int = 17070
)

// Config holds an immutable environment configuration.
type Config struct {
	// m holds the attributes that are defined for Config.
	// t holds the other attributes that are passed in (aka UnknownAttrs).
	// the union of these two are AllAttrs
	m, t map[string]interface{}
}

// Defaulting is a value that specifies whether a configuration
// creator should use defaults from the environment.
type Defaulting bool

const (
	UseDefaults Defaulting = true
	NoDefaults  Defaulting = false
)

// TODO(rog) update the doc comment below - it's getting messy
// and it assumes too much prior knowledge.

// New returns a new configuration.  Fields that are common to all
// environment providers are verified.  If useDefaults is UseDefaults,
// default values will be taken from the environment.
//
// Specifically, the "authorized-keys-path" key
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
func New(withDefaults Defaulting, attrs map[string]interface{}) (*Config, error) {
	checker := noDefaultsChecker
	if withDefaults {
		checker = withDefaultsChecker
	}
	m, err := checker.Coerce(attrs, nil)
	if err != nil {
		return nil, err
	}
	c := &Config{
		m: m.(map[string]interface{}),
		t: make(map[string]interface{}),
	}
	if withDefaults {
		if err := c.fillInDefaults(); err != nil {
			return nil, err
		}
	}
	// no old config to compare against
	if err = Validate(c, nil); err != nil {
		return nil, err
	}
	// Copy unknown attributes onto the type-specific map.
	for k, v := range attrs {
		if _, ok := fields[k]; !ok {
			c.t[k] = v
		}
	}
	return c, nil
}

func (c *Config) fillInDefaults() error {
	// For backward compatibility purposes, we treat as unset string
	// valued attributes that are set to the empty string, and fill
	// out their defaults accordingly.
	c.fillInStringDefault("default-series")
	c.fillInStringDefault("firewall-mode")

	// Load authorized-keys-path into authorized-keys if necessary.
	path := c.asString("authorized-keys-path")
	keys := c.asString("authorized-keys")
	if path != "" || keys == "" {
		var err error
		c.m["authorized-keys"], err = readAuthorizedKeys(path)
		if err != nil {
			return err
		}
	}
	delete(c.m, "authorized-keys-path")

	// Don't use c.Name() because the name hasn't
	// been verified yet.
	name := c.asString("name")
	if name == "" {
		return fmt.Errorf("empty name in environment configuration")
	}
	err := maybeReadAttrFromFile(c.m, "ca-cert", name+"-cert.pem")
	if err != nil {
		return err
	}
	err = maybeReadAttrFromFile(c.m, "ca-private-key", name+"-private-key.pem")
	if err != nil {
		return err
	}
	return nil
}

func (c *Config) fillInStringDefault(attr string) {
	if c.asString(attr) == "" {
		c.m[attr] = defaults[attr]
	}
}

// Validate ensures that config is a valid configuration.  If old is not nil,
// it holds the previous environment configuration for consideration when
// validating changes.
func Validate(cfg, old *Config) error {
	// Check that we don't have any disallowed fields.
	for _, attr := range allowedWithDefaultsOnly {
		if _, ok := cfg.m[attr]; ok {
			return fmt.Errorf("attribute %q is not allowed in configuration", attr)
		}
	}
	// Check that mandatory fields are specified.
	for _, attr := range mandatoryWithoutDefaults {
		if _, ok := cfg.m[attr]; !ok {
			return fmt.Errorf("%s missing from environment configuration", attr)
		}
	}

	// Check that all other fields that have been specified are non-empty.
	for attr, val := range cfg.m {
		if isEmpty(val) {
			return fmt.Errorf("empty %s in environment configuration", attr)
		}
	}

	if strings.ContainsAny(cfg.mustString("name"), "/\\") {
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
	if mode := cfg.FirewallMode(); mode != FwInstance && mode != FwGlobal {
		return fmt.Errorf("invalid firewall mode in environment configuration: %q", mode)
	}

	caCert, caCertOK := cfg.CACert()
	caKey, caKeyOK := cfg.CAPrivateKey()
	if caCertOK || caKeyOK {
		if err := verifyKeyPair(caCert, caKey); err != nil {
			return fmt.Errorf("bad CA certificate/key in configuration: %v", err)
		}
	}

	// Check the immutable config values.  These can't change
	if old != nil {
		for _, attr := range immutableAttributes {
			if newv, oldv := cfg.m[attr], old.m[attr]; newv != oldv {
				return fmt.Errorf("cannot change %s from %#v to %#v", attr, oldv, newv)
			}
		}
		if _, oldFound := old.AgentVersion(); oldFound {
			if _, newFound := cfg.AgentVersion(); !newFound {
				return fmt.Errorf("cannot clear agent-version")
			}
		}
	}
	return nil
}

func isEmpty(val interface{}) bool {
	switch val := val.(type) {
	case nil:
		return true
	case bool:
		return false
	case int:
		return val == 0
	case string:
		return val == ""
	}
	panic(fmt.Errorf("unexpected type %T in configuration", val))
}

// maybeReadAttrFromFile sets m[attr] to:
//
// 1) The content of the file m[attr+"-path"], if that's set
// 2) The value of m[attr] if it is already set.
// 3) The content of defaultPath if it exists and m[attr] is unset
// 4) Preserves the content of m[attr], otherwise
//
// The m[attr+"-path"] key is always deleted.
func maybeReadAttrFromFile(m map[string]interface{}, attr, defaultPath string) error {
	pathAttr := attr + "-path"
	path, _ := m[pathAttr].(string)
	delete(m, pathAttr)
	hasPath := path != ""
	if !hasPath {
		// No path and attribute is already set; leave it be.
		if _, ok := m[attr]; ok {
			return nil
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
			// If the default path isn't found, it's
			// not an error.
			return nil
		}
		return err
	}
	if len(data) == 0 {
		return fmt.Errorf("file %q is empty", path)
	}
	m[attr] = string(data)
	return nil
}

// asString is a private helper method to keep the ugly string casting
// in once place. It returns the given named attribute as a string,
// returning "" if it isn't found.
func (c *Config) asString(name string) string {
	value, _ := c.m[name].(string)
	return value
}

// mustString returns the named attribute as an string, panicking if
// it is not found or is empty.
func (c *Config) mustString(name string) string {
	value, _ := c.m[name].(string)
	if value == "" {
		panic(fmt.Errorf("empty value for %q found in configuration (type %T, val %v)", name, c.m[name], c.m[name]))
	}
	return value
}

// mustInt returns the named attribute as an integer, panicking if
// it is not found or is zero.
func (c *Config) mustInt(name string) int {
	value, _ := c.m[name].(int)
	if value == 0 {
		panic(fmt.Errorf("empty value for %q found in configuration", name))
	}
	return value
}

// Type returns the environment type.
func (c *Config) Type() string {
	return c.mustString("type")
}

// Name returns the environment name.
func (c *Config) Name() string {
	return c.mustString("name")
}

// DefaultSeries returns the default Ubuntu series for the environment.
func (c *Config) DefaultSeries() string {
	return c.mustString("default-series")
}

// StatePort returns the state server port for the environment.
func (c *Config) StatePort() int {
	return c.mustInt("state-port")
}

// APIPort returns the API server port for the environment.
func (c *Config) APIPort() int {
	return c.mustInt("api-port")
}

// AuthorizedKeys returns the content for ssh's authorized_keys file.
func (c *Config) AuthorizedKeys() string {
	return c.mustString("authorized-keys")
}

// CACert returns the certificate of the CA that signed the state server
// certificate, in PEM format, and whether the setting is available.
func (c *Config) CACert() ([]byte, bool) {
	if s, ok := c.m["ca-cert"]; ok {
		return []byte(s.(string)), true
	}
	return nil, false
}

// CAPrivateKey returns the private key of the CA that signed the state
// server certificate, in PEM format, and whether the setting is available.
func (c *Config) CAPrivateKey() (key []byte, ok bool) {
	if s, ok := c.m["ca-private-key"]; ok {
		return []byte(s.(string)), true
	}
	return nil, false
}

// AdminSecret returns the administrator password.
// It's empty if the password has not been set.
func (c *Config) AdminSecret() string {
	if s, ok := c.m["admin-secret"]; ok {
		return s.(string)
	}
	return ""
}

// FirewallMode returns whether the firewall should
// manage ports per machine or global
// (FwInstance or FwGlobal)
func (c *Config) FirewallMode() string {
	return c.mustString("firewall-mode")
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

// ToolsURL returns the URL that locates the tools tarballs and metadata,
// and whether it has been set.
func (c *Config) ToolsURL() (string, bool) {
	if url, ok := c.m["tools-url"]; ok {
		return url.(string), true
	}
	return "", false
}

// ImageMetadataURL returns the URL at which the metadata used to locate image ids is located,
// and wether it has been set.
func (c *Config) ImageMetadataURL() (string, bool) {
	if url, ok := c.m["image-metadata-url"]; ok {
		return url.(string), true
	}
	return "", false
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
	return New(NoDefaults, m)
}

var fields = schema.Fields{
	"type":                      schema.String(),
	"name":                      schema.String(),
	"default-series":            schema.String(),
	"tools-url":                 schema.String(),
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

// alwaysOptional holds configuration defaults
// for attributes that may be unspecified even
// after a configuration has been created with
// all defaults filled out.
var alwaysOptional = schema.Defaults{
	"agent-version":        schema.Omit,
	"admin-secret":         schema.Omit,
	"ca-cert":              schema.Omit,
	"ca-private-key":       schema.Omit,
	"image-metadata-url":   schema.Omit,
	"tools-url":            schema.Omit,
	"authorized-keys":      schema.Omit,
	"authorized-keys-path": schema.Omit,
	"ca-cert-path":         schema.Omit,
	"ca-private-key-path":  schema.Omit,
}

var defaults = allDefaults()

func allDefaults() schema.Defaults {
	d := schema.Defaults{
		"default-series":            DefaultSeries,
		"firewall-mode":             FwInstance,
		"development":               false,
		"ssl-hostname-verification": true,
		"state-port":                DefaultStatePort,
		"api-port":                  DefaultAPIPort,
	}
	for attr, val := range alwaysOptional {
		d[attr] = val
	}
	return d
}

// allowedWithDefaultsOnly holds those attributes
// that are only allowed in a configuration that is
// being created with UseDefaults.
var allowedWithDefaultsOnly = []string{
	"ca-cert-path",
	"ca-private-key-path",
	"authorized-keys-path",
}

// mandatoryWithoutDefaults holds those attributes
// that are mandatory if the configuration is created
// with no defaults but optional otherwise.
var mandatoryWithoutDefaults = []string{
	"authorized-keys",
}

// immutableAttributes holds those attributes
// which are not allowed to change in the lifetime
// of an environment.
var immutableAttributes = []string{
	"name",
	"type",
	"firewall-mode",
	"state-port",
	"api-port",
}
var (
	withDefaultsChecker = schema.FieldMap(fields, defaults)
	noDefaultsChecker   = schema.FieldMap(fields, alwaysOptional)
)

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
