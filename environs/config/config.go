// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"launchpad.net/loggo"

	"launchpad.net/juju-core/cert"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/juju/osenv"
	"launchpad.net/juju-core/schema"
	"launchpad.net/juju-core/utils"
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

	// DefaultSyslogPort is the default port that the syslog UDP listener is
	// listening on.
	DefaultSyslogPort int = 514
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
// recognised are "agent-version" (string) and "development" (bool) as
// well as charm-store-auth (string containing comma-separated key=value pairs).
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
	if err := c.ensureUnitLogging(); err != nil {
		return nil, err
	}
	// no old config to compare against
	if err := Validate(c, nil); err != nil {
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

func (c *Config) ensureUnitLogging() error {
	loggingConfig := c.asString("logging-config")
	// If the logging config hasn't been set, then look for the os environment
	// variable, and failing that, get the config from loggo itself.
	if loggingConfig == "" {
		if environmentValue := os.Getenv(osenv.JujuLoggingConfigEnvKey); environmentValue != "" {
			loggingConfig = environmentValue
		} else {
			loggingConfig = loggo.LoggerInfo()
		}
	}
	levels, err := loggo.ParseConfigurationString(loggingConfig)
	if err != nil {
		return err
	}
	// If there is is no specified level for "unit", then set one.
	if _, ok := levels["unit"]; !ok {
		loggingConfig = loggingConfig + ";unit=DEBUG"
	}
	c.m["logging-config"] = loggingConfig
	return nil
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
		c.m["authorized-keys"], err = ReadAuthorizedKeys(path)
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

// processDeprecatedAttributes ensures that the config is set up so that it works
// correctly when used with older versions of Juju which require that deprecated
// attribute values still be used.
func (cfg *Config) processDeprecatedAttributes() {
	// The tools url has changed so ensure that both old and new values are in the config so that
	// upgrades work. "tools-url" is the old attribute name.
	if oldToolsURL := cfg.m["tools-url"]; oldToolsURL != nil && oldToolsURL.(string) != "" {
		_, newToolsSpecified := cfg.ToolsURL()
		// Ensure the new attribute name "tools-metadata-url" is set.
		if !newToolsSpecified {
			cfg.m["tools-metadata-url"] = oldToolsURL
		}
	}
	// Even if the user has edited their environment yaml to remove the deprecated tools-url value,
	// we still want it in the config for upgrades.
	cfg.m["tools-url"], _ = cfg.ToolsURL()
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

	// Check that all other fields that have been specified are non-empty,
	// unless they're allowed to be empty for backward compatibility,
	for attr, val := range cfg.m {
		if !isEmpty(val) {
			continue
		}
		if !allowEmpty(attr) {
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

	// If the logging config is set, make sure it is valid.
	if v, ok := cfg.m["logging-config"].(string); ok {
		if _, err := loggo.ParseConfigurationString(v); err != nil {
			return err
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

	// Ensure that the auth token is a set of key=value pairs.
	authToken, _ := cfg.CharmStoreAuth()
	validAuthToken := regexp.MustCompile(`^([^\s=]+=[^\s=]+(,\s*)?)*$`)
	if !validAuthToken.MatchString(authToken) {
		return fmt.Errorf("charm store auth token needs to be a set"+
			" of key-value pairs, not %q", authToken)
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

	cfg.processDeprecatedAttributes()
	return nil
}

func isEmpty(val interface{}) bool {
	switch val := val.(type) {
	case nil:
		return true
	case bool:
		return false
	case int:
		// TODO(rog) fix this to return false when
		// we can lose backward compatibility.
		// https://bugs.launchpad.net/juju-core/+bug/1224492
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
		if s, _ := m[attr].(string); s != "" {
			return nil
		}
		path = defaultPath
	}
	path, err := utils.NormalizePath(path)
	if err != nil {
		return err
	}
	if !filepath.IsAbs(path) {
		path = osenv.JujuHomePath(path)
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
// it is not found or is zero. Zero values should have been
// diagnosed at Validate time.
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

// SyslogPort returns the syslog port for the environment.
func (c *Config) SyslogPort() int {
	return c.mustInt("syslog-port")
}

// AuthorizedKeys returns the content for ssh's authorized_keys file.
func (c *Config) AuthorizedKeys() string {
	return c.mustString("authorized-keys")
}

// ProxySettings returns all three proxy settings; http, https and ftp.
func (c *Config) ProxySettings() osenv.ProxySettings {
	return osenv.ProxySettings{
		Http:  c.HttpProxy(),
		Https: c.HttpsProxy(),
		Ftp:   c.FtpProxy(),
	}
}

// HttpProxy returns the http proxy for the environment.
func (c *Config) HttpProxy() string {
	return c.asString("http-proxy")
}

// HttpsProxy returns the https proxy for the environment.
func (c *Config) HttpsProxy() string {
	return c.asString("https-proxy")
}

// FtpProxy returns the ftp proxy for the environment.
func (c *Config) FtpProxy() string {
	return c.asString("ftp-proxy")
}

func (c *Config) getWithFallback(key, fallback string) string {
	value := c.asString(key)
	if value == "" {
		value = c.asString(fallback)
	}
	return value
}

// AptProxySettings returns all three proxy settings; http, https and ftp.
func (c *Config) AptProxySettings() osenv.ProxySettings {
	return osenv.ProxySettings{
		Http:  c.AptHttpProxy(),
		Https: c.AptHttpsProxy(),
		Ftp:   c.AptFtpProxy(),
	}
}

// AptHttpProxy returns the apt http proxy for the environment.
// Falls back to the default http-proxy if not specified.
func (c *Config) AptHttpProxy() string {
	return c.getWithFallback("apt-http-proxy", "http-proxy")
}

// AptHttpsProxy returns the apt https proxy for the environment.
// Falls back to the default https-proxy if not specified.
func (c *Config) AptHttpsProxy() string {
	return c.getWithFallback("apt-https-proxy", "https-proxy")
}

// AptFtpProxy returns the apt ftp proxy for the environment.
// Falls back to the default ftp-proxy if not specified.
func (c *Config) AptFtpProxy() string {
	return c.getWithFallback("apt-ftp-proxy", "ftp-proxy")
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
	if s, ok := c.m["ca-private-key"]; ok && s != "" {
		return []byte(s.(string)), true
	}
	return nil, false
}

// AdminSecret returns the administrator password.
// It's empty if the password has not been set.
func (c *Config) AdminSecret() string {
	if s, ok := c.m["admin-secret"]; ok && s != "" {
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
	if url, ok := c.m["tools-metadata-url"]; ok && url != "" {
		return url.(string), true
	}
	return "", false
}

// ImageMetadataURL returns the URL at which the metadata used to locate image ids is located,
// and wether it has been set.
func (c *Config) ImageMetadataURL() (string, bool) {
	if url, ok := c.m["image-metadata-url"]; ok && url != "" {
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

// LoggingConfig returns the configuration string for the loggers.
func (c *Config) LoggingConfig() string {
	return c.asString("logging-config")
}

// Auth token sent to charm store
func (c *Config) CharmStoreAuth() (string, bool) {
	auth := c.asString("charm-store-auth")
	return auth, auth != ""
}

// ProvisionerSafeMode reports whether the provisioner should not
// destroy machines it does not know about.
func (c *Config) ProvisionerSafeMode() bool {
	v, _ := c.m["provisioner-safe-mode"].(bool)
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
	return New(NoDefaults, m)
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
	"syslog-port":               schema.ForceInt(),
	"logging-config":            schema.String(),
	"charm-store-auth":          schema.String(),
	"provisioner-safe-mode":     schema.Bool(),
	"http-proxy":                schema.String(),
	"https-proxy":               schema.String(),
	"ftp-proxy":                 schema.String(),
	"apt-http-proxy":            schema.String(),
	"apt-https-proxy":           schema.String(),
	"apt-ftp-proxy":             schema.String(),

	// Deprecated fields, retain for backwards compatibility.
	"tools-url": schema.String(),
}

// alwaysOptional holds configuration defaults for attributes that may
// be unspecified even after a configuration has been created with all
// defaults filled out.
//
// This table is not definitive: it specifies those attributes which are
// optional when the config goes through its initial schema coercion,
// but some fields listed as optional here are actually mandatory
// with NoDefaults and are checked at the later Validate stage.
var alwaysOptional = schema.Defaults{
	"agent-version":         schema.Omit,
	"ca-cert":               schema.Omit,
	"authorized-keys":       schema.Omit,
	"authorized-keys-path":  schema.Omit,
	"ca-cert-path":          schema.Omit,
	"ca-private-key-path":   schema.Omit,
	"logging-config":        schema.Omit,
	"provisioner-safe-mode": schema.Omit,
	"http-proxy":            schema.Omit,
	"https-proxy":           schema.Omit,
	"ftp-proxy":             schema.Omit,
	"apt-http-proxy":        schema.Omit,
	"apt-https-proxy":       schema.Omit,
	"apt-ftp-proxy":         schema.Omit,

	// Deprecated fields, retain for backwards compatibility.
	"tools-url": "",

	// For backward compatibility reasons, the following
	// attributes default to empty strings rather than being
	// omitted.
	// TODO(rog) remove this support when we can
	// remove upgrade compatibility with versions prior to 1.14.
	"admin-secret":       "", // TODO(rog) omit
	"ca-private-key":     "", // TODO(rog) omit
	"image-metadata-url": "", // TODO(rog) omit
	"tools-metadata-url": "", // TODO(rog) omit

	// For backward compatibility only - default ports were
	// not filled out in previous versions of the configuration.
	"state-port":  DefaultStatePort,
	"api-port":    DefaultAPIPort,
	"syslog-port": DefaultSyslogPort,
	// Authentication string sent with requests to the charm store
	"charm-store-auth": "",
}

func allowEmpty(attr string) bool {
	return alwaysOptional[attr] == ""
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
		"syslog-port":               DefaultSyslogPort,
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
	"syslog-port",
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
		logger.Debugf("coercion failed attributes: %#v, checker: %#v, %v", attrs, checker, err)
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
	var noHostnames []string
	return cert.NewServer(caCert, caKey, time.Now().UTC().AddDate(10, 0, 0), noHostnames)
}

type Authorizer interface {
	WithAuthAttrs(string) charm.Repository
}

// AuthorizeCharmRepo returns a repository with authentication added
// from the specified configuration.
func AuthorizeCharmRepo(repo charm.Repository, cfg *Config) charm.Repository {
	// If a charm store auth token is set, pass it on to the charm store
	if auth, authSet := cfg.CharmStoreAuth(); authSet {
		if CS, isCS := repo.(Authorizer); isCS {
			repo = CS.WithAuthAttrs(auth)
		}
	}
	return repo
}
