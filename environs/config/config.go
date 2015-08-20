// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/schema"
	"github.com/juju/utils"
	"github.com/juju/utils/proxy"
	"gopkg.in/juju/charm.v5"
	"gopkg.in/juju/charm.v5/charmrepo"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/cert"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/version"
)

var logger = loggo.GetLogger("juju.environs.config")

const (
	// FwInstance requests the use of an individual firewall per instance.
	FwInstance = "instance"

	// FwGlobal requests the use of a single firewall group for all machines.
	// When ports are opened for one machine, all machines will have the same
	// port opened.
	FwGlobal = "global"

	// FwNone requests that no firewalling should be performed inside
	// the environment. No firewaller worker will be started. It's
	// useful for clouds without support for either global or per
	// instance security groups.
	FwNone = "none"

	// DefaultStatePort is the default port the state server is listening on.
	DefaultStatePort int = 37017

	// DefaultApiPort is the default port the API server is listening on.
	DefaultAPIPort int = 17070

	// DefaultSyslogPort is the default port that the syslog UDP/TCP listener is
	// listening on.
	DefaultSyslogPort int = 6514

	// DefaultBootstrapSSHTimeout is the amount of time to wait
	// contacting a state server, in seconds.
	DefaultBootstrapSSHTimeout int = 600

	// DefaultBootstrapSSHRetryDelay is the amount of time between
	// attempts to connect to an address, in seconds.
	DefaultBootstrapSSHRetryDelay int = 5

	// DefaultBootstrapSSHAddressesDelay is the amount of time between
	// refreshing the addresses, in seconds. Not too frequent, as we
	// refresh addresses from the provider each time.
	DefaultBootstrapSSHAddressesDelay int = 10

	// fallbackLtsSeries is the latest LTS series we'll use, if we fail to
	// obtain this information from the system.
	fallbackLtsSeries string = "trusty"

	// DefaultNumaControlPolicy should not be used by default.
	// Only use numactl if user specifically requests it
	DefaultNumaControlPolicy = false

	// DefaultPreventDestroyEnvironment should not be used by default.
	// Only prevent destroy-environment from running
	// if user specifically requests it. Otherwise, let it run.
	DefaultPreventDestroyEnvironment = false

	// DefaultPreventRemoveObject should not be used by default.
	// Only prevent remove-object from running
	// if user specifically requests it. Otherwise, let it run.
	// Object here is a juju artifact - machine, service, unit or relation.
	DefaultPreventRemoveObject = false

	// DefaultPreventAllChanges should not be used by default.
	// Only prevent all-changes from running
	// if user specifically requests it. Otherwise, let them run.
	DefaultPreventAllChanges = false

	// DefaultLXCDefaultMTU is the default value for "lxc-default-mtu"
	// config setting. Only non-zero, positive integer values will
	// have effect.
	DefaultLXCDefaultMTU = 0
)

// TODO(katco-): Please grow this over time.
// Centralized place to store values of config keys. This transitions
// mistakes in referencing key-values to a compile-time error.
const (
	//
	// Settings Attributes
	//

	// ProvisionerHarvestModeKey stores the key for this setting.
	ProvisionerHarvestModeKey = "provisioner-harvest-mode"

	// AgentStreamKey stores the key for this setting.
	AgentStreamKey = "agent-stream"

	// AgentMetadataURLKey stores the key for this setting.
	AgentMetadataURLKey = "agent-metadata-url"

	// HttpProxyKey stores the key for this setting.
	HttpProxyKey = "http-proxy"

	// HttpsProxyKey stores the key for this setting.
	HttpsProxyKey = "https-proxy"

	// FtpProxyKey stores the key for this setting.
	FtpProxyKey = "ftp-proxy"

	// AptHttpProxyKey stores the key for this setting.
	AptHttpProxyKey = "apt-http-proxy"

	// AptHttpsProxyKey stores the key for this setting.
	AptHttpsProxyKey = "apt-https-proxy"

	// AptFtpProxyKey stores the key for this setting.
	AptFtpProxyKey = "apt-ftp-proxy"

	// NoProxyKey stores the key for this setting.
	NoProxyKey = "no-proxy"

	// LxcClone stores the value for this setting.
	LxcClone = "lxc-clone"

	// NumaControlPolicyKey stores the value for this setting
	SetNumaControlPolicyKey = "set-numa-control-policy"

	// BlockKeyPrefix is the prefix used for environment variables that block commands
	// TODO(anastasiamac 2015-02-27) remove it and all related post 1.24 as obsolete
	BlockKeyPrefix = "block-"

	// PreventDestroyEnvironmentKey stores the value for this setting
	PreventDestroyEnvironmentKey = BlockKeyPrefix + "destroy-environment"

	// PreventRemoveObjectKey stores the value for this setting
	PreventRemoveObjectKey = BlockKeyPrefix + "remove-object"

	// PreventAllChangesKey stores the value for this setting
	PreventAllChangesKey = BlockKeyPrefix + "all-changes"

	// The default block storage source.
	StorageDefaultBlockSourceKey = "storage-default-block-source"

	// ResourceTagsKey is an optional list or space-separated string
	// of k=v pairs, defining the tags for ResourceTags.
	ResourceTagsKey = "resource-tags"

	// For LXC containers, is the container allowed to mount block
	// devices. A theoretical security issue, so must be explicitly
	// allowed by the user.
	AllowLXCLoopMounts = "allow-lxc-loop-mounts"

	// LXCDefaultMTU, when set to a positive integer, overrides the
	// Machine Transmission Unit (MTU) setting of all network
	// interfaces created for LXC containers. See also bug #1442257.
	LXCDefaultMTU = "lxc-default-mtu"

	//
	// Deprecated Settings Attributes
	//

	// Deprecated by provisioner-harvest-mode
	// ProvisionerSafeModeKey stores the key for this setting.
	ProvisionerSafeModeKey = "provisioner-safe-mode"

	// Deprecated by agent-stream
	// ToolsStreamKey stores the key for this setting.
	ToolsStreamKey = "tools-stream"

	// Deprecated by agent-metadata-url
	// ToolsMetadataURLKey stores the key for this setting.
	ToolsMetadataURLKey = "tools-metadata-url"

	// Deprecated by use-clone
	// LxcUseClone stores the key for this setting.
	LxcUseClone = "lxc-use-clone"

	// IgnoreMachineAddresses, when true, will cause the
	// machine worker not to discover any machine addresses
	// on start up.
	IgnoreMachineAddresses = "ignore-machine-addresses"
)

// ParseHarvestMode parses description of harvesting method and
// returns the representation.
func ParseHarvestMode(description string) (HarvestMode, error) {
	description = strings.ToLower(description)
	for method, descr := range harvestingMethodToFlag {
		if description == descr {
			return method, nil
		}
	}
	return 0, fmt.Errorf("unknown harvesting method: %s", description)
}

// HarvestMode is a bit field which is used to store the harvesting
// behavior for Juju.
type HarvestMode uint32

const (
	// HarvestNone signifies that Juju should not harvest any
	// machines.
	HarvestNone HarvestMode = 1 << iota
	// HarvestUnknown signifies that Juju should only harvest machines
	// which exist, but we don't know about.
	HarvestUnknown
	// HarvestDestroyed signifies that Juju should only harvest
	// machines which have been explicitly released by the user
	// through a destroy of a service/environment/unit.
	HarvestDestroyed
	// HarvestAll signifies that Juju should harvest both unknown and
	// destroyed instances. ♫ Don't fear the reaper. ♫
	HarvestAll HarvestMode = HarvestUnknown | HarvestDestroyed
)

// A mapping from method to description. Going this way will be the
// more common operation, so we want this type of lookup to be O(1).
var harvestingMethodToFlag = map[HarvestMode]string{
	HarvestAll:       "all",
	HarvestNone:      "none",
	HarvestUnknown:   "unknown",
	HarvestDestroyed: "destroyed",
}

// proxyAttrs contains attribute names that could contain loopback URLs, pointing to localhost
var ProxyAttributes = []string{
	HttpProxyKey,
	HttpsProxyKey,
	FtpProxyKey,
	AptHttpProxyKey,
	AptHttpsProxyKey,
	AptFtpProxyKey,
}

// String returns the description of the harvesting mode.
func (method HarvestMode) String() string {
	if description, ok := harvestingMethodToFlag[method]; ok {
		return description
	}
	panic("Unknown harvesting method.")
}

// None returns whether or not the None harvesting flag is set.
func (method HarvestMode) HarvestNone() bool {
	return method&HarvestNone != 0
}

// Destroyed returns whether or not the Destroyed harvesting flag is set.
func (method HarvestMode) HarvestDestroyed() bool {
	return method&HarvestDestroyed != 0
}

// Unknown returns whether or not the Unknown harvesting flag is set.
func (method HarvestMode) HarvestUnknown() bool {
	return method&HarvestUnknown != 0
}

var latestLtsSeries string

type HasDefaultSeries interface {
	DefaultSeries() (string, bool)
}

// PreferredSeries returns the preferred series to use when a charm does not
// explicitly specify a series.
func PreferredSeries(cfg HasDefaultSeries) string {
	if series, ok := cfg.DefaultSeries(); ok {
		return series
	}
	return LatestLtsSeries()
}

func LatestLtsSeries() string {
	if latestLtsSeries == "" {
		series, err := distroLtsSeries()
		if err != nil {
			latestLtsSeries = fallbackLtsSeries
		} else {
			latestLtsSeries = series
		}
	}
	return latestLtsSeries
}

var distroLtsSeries = distroLtsSeriesFunc

// distroLtsSeriesFunc returns the latest LTS series, if this information is
// available on this system.
func distroLtsSeriesFunc() (string, error) {
	out, err := exec.Command("distro-info", "--lts").Output()
	if err != nil {
		return "", err
	}
	series := strings.TrimSpace(string(out))
	if !charm.IsValidSeries(series) {
		return "", fmt.Errorf("not a valid LTS series: %q", series)
	}
	return series, nil
}

// Config holds an immutable environment configuration.
type Config struct {
	// defined holds the attributes that are defined for Config.
	// unknown holds the other attributes that are passed in (aka UnknownAttrs).
	// the union of these two are AllAttrs
	defined, unknown map[string]interface{}
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
// recognised are "agent-version" (string) and "development" (bool).
func New(withDefaults Defaulting, attrs map[string]interface{}) (*Config, error) {
	checker := noDefaultsChecker
	if withDefaults {
		checker = withDefaultsChecker
	}
	defined, err := checker.Coerce(attrs, nil)
	if err != nil {
		return nil, err
	}
	c := &Config{
		defined: defined.(map[string]interface{}),
		unknown: make(map[string]interface{}),
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
			c.unknown[k] = v
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
	c.defined["logging-config"] = loggingConfig
	return nil
}

func (c *Config) fillInDefaults() error {
	// For backward compatibility purposes, we treat as unset string
	// valued attributes that are set to the empty string, and fill
	// out their defaults accordingly.
	c.fillInStringDefault("firewall-mode")

	// Load authorized-keys-path into authorized-keys if necessary.
	path := c.asString("authorized-keys-path")
	keys := c.asString("authorized-keys")
	if path != "" || keys == "" {
		var err error
		c.defined["authorized-keys"], err = ReadAuthorizedKeys(path)
		if err != nil {
			return err
		}
	}
	delete(c.defined, "authorized-keys-path")

	// Don't use c.Name() because the name hasn't
	// been verified yet.
	name := c.asString("name")
	if name == "" {
		return fmt.Errorf("empty name in environment configuration")
	}
	err := maybeReadAttrFromFile(c.defined, "ca-cert", name+"-cert.pem")
	if err != nil {
		return err
	}
	err = maybeReadAttrFromFile(c.defined, "ca-private-key", name+"-private-key.pem")
	if err != nil {
		return err
	}
	return nil
}

func (c *Config) fillInStringDefault(attr string) {
	if c.asString(attr) == "" {
		c.defined[attr] = defaults[attr]
	}
}

// ProcessDeprecatedAttributes gathers any deprecated attributes in attrs and adds or replaces
// them with new name value pairs for the replacement attrs.
// Ths ensures that older versions of Juju which require that deprecated
// attribute values still be used will work as expected.
func ProcessDeprecatedAttributes(attrs map[string]interface{}) map[string]interface{} {
	processedAttrs := make(map[string]interface{}, len(attrs))
	for k, v := range attrs {
		processedAttrs[k] = v
	}
	// The tools url has changed so ensure that both old and new values are in the config so that
	// upgrades work. "agent-metadata-url" is the old attribute name.
	if oldToolsURL, ok := attrs[ToolsMetadataURLKey]; ok && oldToolsURL.(string) != "" {
		if newTools, ok := attrs[AgentMetadataURLKey]; !ok || newTools.(string) == "" {
			// Ensure the new attribute name "agent-metadata-url" is set.
			processedAttrs[AgentMetadataURLKey] = oldToolsURL
		}
		// Even if the user has edited their environment yaml to remove the deprecated tools-metadata-url value,
		// we still want it in the config for upgrades.
		processedAttrs[ToolsMetadataURLKey] = processedAttrs[AgentMetadataURLKey]
	}

	// Copy across lxc-use-clone to lxc-clone.
	if lxcUseClone, ok := attrs[LxcUseClone]; ok {
		_, newValSpecified := attrs[LxcClone]
		// Ensure the new attribute name "lxc-clone" is set.
		if !newValSpecified {
			processedAttrs[LxcClone] = lxcUseClone
		}
	}

	// Update the provider type from null to manual.
	if attrs["type"] == "null" {
		processedAttrs["type"] = "manual"
	}

	if _, ok := attrs[ProvisionerHarvestModeKey]; !ok {
		if safeMode, ok := attrs[ProvisionerSafeModeKey].(bool); ok {

			var harvestModeDescr string
			if safeMode {
				harvestModeDescr = HarvestDestroyed.String()
			} else {
				harvestModeDescr = HarvestAll.String()
			}

			processedAttrs[ProvisionerHarvestModeKey] = harvestModeDescr

			logger.Infof(
				`Based on your "%s" setting, configuring "%s" to "%s".`,
				ProvisionerSafeModeKey,
				ProvisionerHarvestModeKey,
				harvestModeDescr,
			)
		}
	}

	// Update agent-stream from tools-stream if agent-stream was not specified but tools-stream was.
	if _, ok := attrs[AgentStreamKey]; !ok {
		if toolsKey, ok := attrs[ToolsStreamKey]; ok {
			processedAttrs[AgentStreamKey] = toolsKey
			logger.Infof(
				`Based on your "%s" setting, configuring "%s" to "%s".`,
				ToolsStreamKey,
				AgentStreamKey,
				toolsKey,
			)
		}
	}
	return processedAttrs
}

// InvalidConfigValue is an error type for a config value that failed validation.
type InvalidConfigValueError struct {
	// Key is the config key used to access the value.
	Key string
	// Value is the value that failed validation.
	Value string
	// Reason indicates why the value failed validation.
	Reason error
}

// Error returns the error string.
func (e *InvalidConfigValueError) Error() string {
	msg := fmt.Sprintf("invalid config value for %s: %q", e.Key, e.Value)
	if e.Reason != nil {
		msg = msg + ": " + e.Reason.Error()
	}
	return msg
}

// Validate ensures that config is a valid configuration.  If old is not nil,
// it holds the previous environment configuration for consideration when
// validating changes.
func Validate(cfg, old *Config) error {
	// Check that we don't have any disallowed fields.
	for _, attr := range allowedWithDefaultsOnly {
		if _, ok := cfg.defined[attr]; ok {
			return fmt.Errorf("attribute %q is not allowed in configuration", attr)
		}
	}
	// Check that mandatory fields are specified.
	for _, attr := range mandatoryWithoutDefaults {
		if _, ok := cfg.defined[attr]; !ok {
			return fmt.Errorf("%s missing from environment configuration", attr)
		}
	}

	// Check that all other fields that have been specified are non-empty,
	// unless they're allowed to be empty for backward compatibility,
	for attr, val := range cfg.defined {
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
	if v, ok := cfg.defined["agent-version"].(string); ok {
		if _, err := version.Parse(v); err != nil {
			return fmt.Errorf("invalid agent version in environment configuration: %q", v)
		}
	}

	// If the logging config is set, make sure it is valid.
	if v, ok := cfg.defined["logging-config"].(string); ok {
		if _, err := loggo.ParseConfigurationString(v); err != nil {
			return err
		}
	}

	caCert, caCertOK := cfg.CACert()
	caKey, caKeyOK := cfg.CAPrivateKey()
	if caCertOK || caKeyOK {
		if err := verifyKeyPair(caCert, caKey); err != nil {
			return errors.Annotate(err, "bad CA certificate/key in configuration")
		}
	}

	if uuid, ok := cfg.defined["uuid"]; ok && !utils.IsValidUUIDString(uuid.(string)) {
		return errors.Errorf("uuid: expected uuid, got string(%q)", uuid)
	}

	// Ensure the resource tags have the expected k=v format.
	if _, err := cfg.resourceTags(); err != nil {
		return errors.Annotate(err, "validating resource tags")
	}

	// Check the immutable config values.  These can't change
	if old != nil {
		for _, attr := range immutableAttributes {
			switch attr {
			case "uuid":
				// uuid is special cased because currently (24/July/2014) there exist no juju
				// environments whose environment configuration's contain a uuid key so we must
				// treat uuid as field that can be updated from non existant to a valid uuid.
				// We do not need to deal with the case of the uuid key being blank as the schema
				// only permits valid uuids in that field.
				oldv, oldexists := old.defined[attr]
				newv := cfg.defined[attr]
				if oldexists && oldv != newv {
					newv := cfg.defined[attr]
					return fmt.Errorf("cannot change %s from %#v to %#v", attr, oldv, newv)
				}
			default:
				if newv, oldv := cfg.defined[attr], old.defined[attr]; newv != oldv {
					return fmt.Errorf("cannot change %s from %#v to %#v", attr, oldv, newv)
				}
			}
		}
		if _, oldFound := old.AgentVersion(); oldFound {
			if _, newFound := cfg.AgentVersion(); !newFound {
				return fmt.Errorf("cannot clear agent-version")
			}
		}
	}

	// Check LXCDefaultMTU is a positive integer, when set.
	if lxcDefaultMTU, ok := cfg.LXCDefaultMTU(); ok && lxcDefaultMTU < 0 {
		return errors.Errorf("%s: expected positive integer, got %v", LXCDefaultMTU, lxcDefaultMTU)
	}

	cfg.defined = ProcessDeprecatedAttributes(cfg.defined)
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
	case []interface{}:
		return len(val) == 0
	case map[string]string:
		return len(val) == 0
	}
	panic(fmt.Errorf("unexpected type %T in configuration", val))
}

// maybeReadAttrFromFile sets defined[attr] to:
//
// 1) The content of the file defined[attr+"-path"], if that's set
// 2) The value of defined[attr] if it is already set.
// 3) The content of defaultPath if it exists and defined[attr] is unset
// 4) Preserves the content of defined[attr], otherwise
//
// The defined[attr+"-path"] key is always deleted.
func maybeReadAttrFromFile(defined map[string]interface{}, attr, defaultPath string) error {
	if !osenv.IsJujuHomeSet() {
		logger.Debugf("JUJU_HOME not set, not attempting to read file %q", defaultPath)
		return nil
	}
	pathAttr := attr + "-path"
	path, _ := defined[pathAttr].(string)
	delete(defined, pathAttr)
	hasPath := path != ""
	if !hasPath {
		// No path and attribute is already set; leave it be.
		if s, _ := defined[attr].(string); s != "" {
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
	defined[attr] = string(data)
	return nil
}

// asString is a private helper method to keep the ugly string casting
// in once place. It returns the given named attribute as a string,
// returning "" if it isn't found.
func (c *Config) asString(name string) string {
	value, _ := c.defined[name].(string)
	return value
}

// mustString returns the named attribute as an string, panicking if
// it is not found or is empty.
func (c *Config) mustString(name string) string {
	value, _ := c.defined[name].(string)
	if value == "" {
		panic(fmt.Errorf("empty value for %q found in configuration (type %T, val %v)", name, c.defined[name], c.defined[name]))
	}
	return value
}

// mustInt returns the named attribute as an integer, panicking if
// it is not found or is zero. Zero values should have been
// diagnosed at Validate time.
func (c *Config) mustInt(name string) int {
	value, _ := c.defined[name].(int)
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

// UUID returns the uuid for the environment.
// For backwards compatability with 1.20 and earlier the value may be blank if
// no uuid is present in this configuration. Once all enviroment configurations
// have been upgraded, this relaxation will be dropped. The absence of a uuid
// is indicated by a result of "", false.
func (c *Config) UUID() (string, bool) {
	value, exists := c.defined["uuid"].(string)
	return value, exists
}

// DefaultSeries returns the configured default Ubuntu series for the environment,
// and whether the default series was explicitly configured on the environment.
func (c *Config) DefaultSeries() (string, bool) {
	if s, ok := c.defined["default-series"]; ok {
		if series, ok := s.(string); ok && series != "" {
			return series, true
		} else if !ok {
			logger.Warningf("invalid default-series: %q", s)
		}
	}
	return "", false
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

// NumaCtlPreference returns if numactl is preferred.
func (c *Config) NumaCtlPreference() bool {
	if numa, ok := c.defined[SetNumaControlPolicyKey]; ok {
		return numa.(bool)
	}
	return DefaultNumaControlPolicy
}

// PreventDestroyEnvironment returns if destroy-environment
// should be blocked from proceeding, thus preventing the operation.
func (c *Config) PreventDestroyEnvironment() bool {
	if attrValue, ok := c.defined[PreventDestroyEnvironmentKey]; ok {
		return attrValue.(bool)
	}
	return DefaultPreventDestroyEnvironment
}

// PreventRemoveObject returns if remove-object
// should be blocked from proceeding, thus preventing the operation.
// Object in this context is a juju artifact: either a machine,
// a service, a unit or a relation.
func (c *Config) PreventRemoveObject() bool {
	if attrValue, ok := c.defined[PreventRemoveObjectKey]; ok {
		return attrValue.(bool)
	}
	return DefaultPreventRemoveObject
}

// PreventAllChanges returns if all-changes
// should be blocked from proceeding, thus preventing the operation.
// Changes in this context are any alterations to current environment.
func (c *Config) PreventAllChanges() bool {
	if attrValue, ok := c.defined[PreventAllChangesKey]; ok {
		return attrValue.(bool)
	}
	return DefaultPreventAllChanges
}

// RsyslogCACert returns the certificate of the CA that signed the
// rsyslog certificate, in PEM format, or nil if one hasn't been
// generated yet.
func (c *Config) RsyslogCACert() string {
	if s, ok := c.defined["rsyslog-ca-cert"]; ok {
		return s.(string)
	}
	return ""
}

// RsyslogCAKey returns the key of the CA that signed the
// rsyslog certificate, in PEM format, or nil if one hasn't been
// generated yet.
func (c *Config) RsyslogCAKey() string {
	if s, ok := c.defined["rsyslog-ca-key"]; ok {
		return s.(string)
	}
	return ""
}

// AuthorizedKeys returns the content for ssh's authorized_keys file.
func (c *Config) AuthorizedKeys() string {
	return c.mustString("authorized-keys")
}

// ProxySSH returns a flag indicating whether SSH commands
// should be proxied through the API server.
func (c *Config) ProxySSH() bool {
	value, _ := c.defined["proxy-ssh"].(bool)
	return value
}

// ProxySettings returns all four proxy settings; http, https, ftp, and no
// proxy.
func (c *Config) ProxySettings() proxy.Settings {
	return proxy.Settings{
		Http:    c.HttpProxy(),
		Https:   c.HttpsProxy(),
		Ftp:     c.FtpProxy(),
		NoProxy: c.NoProxy(),
	}
}

// HttpProxy returns the http proxy for the environment.
func (c *Config) HttpProxy() string {
	return c.asString(HttpProxyKey)
}

// HttpsProxy returns the https proxy for the environment.
func (c *Config) HttpsProxy() string {
	return c.asString(HttpsProxyKey)
}

// FtpProxy returns the ftp proxy for the environment.
func (c *Config) FtpProxy() string {
	return c.asString(FtpProxyKey)
}

// NoProxy returns the 'no proxy' for the environment.
func (c *Config) NoProxy() string {
	return c.asString(NoProxyKey)
}

func (c *Config) getWithFallback(key, fallback string) string {
	value := c.asString(key)
	if value == "" {
		value = c.asString(fallback)
	}
	return value
}

// addSchemeIfMissing adds a scheme to a URL if it is missing
func addSchemeIfMissing(defaultScheme string, url string) string {
	if url != "" && !strings.Contains(url, "://") {
		url = defaultScheme + "://" + url
	}
	return url
}

// AptProxySettings returns all three proxy settings; http, https and ftp.
func (c *Config) AptProxySettings() proxy.Settings {
	return proxy.Settings{
		Http:  c.AptHttpProxy(),
		Https: c.AptHttpsProxy(),
		Ftp:   c.AptFtpProxy(),
	}
}

// AptHttpProxy returns the apt http proxy for the environment.
// Falls back to the default http-proxy if not specified.
func (c *Config) AptHttpProxy() string {
	return addSchemeIfMissing("http", c.getWithFallback(AptHttpProxyKey, HttpProxyKey))
}

// AptHttpsProxy returns the apt https proxy for the environment.
// Falls back to the default https-proxy if not specified.
func (c *Config) AptHttpsProxy() string {
	return addSchemeIfMissing("https", c.getWithFallback(AptHttpsProxyKey, HttpsProxyKey))
}

// AptFtpProxy returns the apt ftp proxy for the environment.
// Falls back to the default ftp-proxy if not specified.
func (c *Config) AptFtpProxy() string {
	return addSchemeIfMissing("ftp", c.getWithFallback(AptFtpProxyKey, FtpProxyKey))
}

// AptMirror sets the apt mirror for the environment.
func (c *Config) AptMirror() string {
	return c.asString("apt-mirror")
}

// BootstrapSSHOpts returns the SSH timeout and retry delays used
// during bootstrap.
func (c *Config) BootstrapSSHOpts() SSHTimeoutOpts {
	opts := SSHTimeoutOpts{
		Timeout:        time.Duration(DefaultBootstrapSSHTimeout) * time.Second,
		RetryDelay:     time.Duration(DefaultBootstrapSSHRetryDelay) * time.Second,
		AddressesDelay: time.Duration(DefaultBootstrapSSHAddressesDelay) * time.Second,
	}
	if v, ok := c.defined["bootstrap-timeout"].(int); ok && v != 0 {
		opts.Timeout = time.Duration(v) * time.Second
	}
	if v, ok := c.defined["bootstrap-retry-delay"].(int); ok && v != 0 {
		opts.RetryDelay = time.Duration(v) * time.Second
	}
	if v, ok := c.defined["bootstrap-addresses-delay"].(int); ok && v != 0 {
		opts.AddressesDelay = time.Duration(v) * time.Second
	}
	return opts
}

// CACert returns the certificate of the CA that signed the state server
// certificate, in PEM format, and whether the setting is available.
func (c *Config) CACert() (string, bool) {
	if s, ok := c.defined["ca-cert"]; ok {
		return s.(string), true
	}
	return "", false
}

// CAPrivateKey returns the private key of the CA that signed the state
// server certificate, in PEM format, and whether the setting is available.
func (c *Config) CAPrivateKey() (key string, ok bool) {
	if s, ok := c.defined["ca-private-key"]; ok && s != "" {
		return s.(string), true
	}
	return "", false
}

// AdminSecret returns the administrator password.
// It's empty if the password has not been set.
func (c *Config) AdminSecret() string {
	if s, ok := c.defined["admin-secret"]; ok && s != "" {
		return s.(string)
	}
	return ""
}

// FirewallMode returns whether the firewall should
// manage ports per machine, globally, or not at all.
// (FwInstance, FwGlobal, or FwNone).
func (c *Config) FirewallMode() string {
	return c.mustString("firewall-mode")
}

// AgentVersion returns the proposed version number for the agent tools,
// and whether it has been set. Once an environment is bootstrapped, this
// must always be valid.
func (c *Config) AgentVersion() (version.Number, bool) {
	if v, ok := c.defined["agent-version"].(string); ok {
		n, err := version.Parse(v)
		if err != nil {
			panic(err) // We should have checked it earlier.
		}
		return n, true
	}
	return version.Zero, false
}

// AgentMetadataURL returns the URL that locates the agent tarballs and metadata,
// and whether it has been set.
func (c *Config) AgentMetadataURL() (string, bool) {
	if url, ok := c.defined[AgentMetadataURLKey]; ok && url != "" {
		return url.(string), true
	}
	return "", false
}

// ImageMetadataURL returns the URL at which the metadata used to locate image ids is located,
// and wether it has been set.
func (c *Config) ImageMetadataURL() (string, bool) {
	if url, ok := c.defined["image-metadata-url"]; ok && url != "" {
		return url.(string), true
	}
	return "", false
}

// Development returns whether the environment is in development mode.
func (c *Config) Development() bool {
	return c.defined["development"].(bool)
}

// PreferIPv6 returns whether IPv6 addresses for API endpoints and
// machines will be preferred (when available) over IPv4.
func (c *Config) PreferIPv6() bool {
	v, _ := c.defined["prefer-ipv6"].(bool)
	return v
}

// EnableOSRefreshUpdate returns whether or not newly provisioned
// instances should run their respective OS's update capability.
func (c *Config) EnableOSRefreshUpdate() bool {
	if val, ok := c.defined["enable-os-refresh-update"].(bool); !ok {
		return true
	} else {
		return val
	}
}

// EnableOSUpgrade returns whether or not newly provisioned instances
// should run their respective OS's upgrade capability.
func (c *Config) EnableOSUpgrade() bool {
	if val, ok := c.defined["enable-os-upgrade"].(bool); !ok {
		return true
	} else {
		return val
	}
}

// SSLHostnameVerification returns weather the environment has requested
// SSL hostname verification to be enabled.
func (c *Config) SSLHostnameVerification() bool {
	return c.defined["ssl-hostname-verification"].(bool)
}

// LoggingConfig returns the configuration string for the loggers.
func (c *Config) LoggingConfig() string {
	return c.asString("logging-config")
}

// ProvisionerHarvestMode reports the harvesting methodology the
// provisioner should take.
func (c *Config) ProvisionerHarvestMode() HarvestMode {
	if v, ok := c.defined[ProvisionerHarvestModeKey].(string); ok {
		if method, err := ParseHarvestMode(v); err != nil {
			// This setting should have already been validated. Don't
			// burden the caller with handling any errors.
			panic(err)
		} else {
			return method
		}
	} else {
		return HarvestDestroyed
	}
}

// ImageStream returns the simplestreams stream
// used to identify which image ids to search
// when starting an instance.
func (c *Config) ImageStream() string {
	v, _ := c.defined["image-stream"].(string)
	if v != "" {
		return v
	}
	return "released"
}

// AgentStream returns the simplestreams stream
// used to identify which tools to use when
// when bootstrapping or upgrading an environment.
func (c *Config) AgentStream() string {
	v, _ := c.defined[AgentStreamKey].(string)
	if v != "" {
		return v
	}
	return "released"
}

// TestMode indicates if the environment is intended for testing.
// In this case, accessing the charm store does not affect statistical
// data of the store.
func (c *Config) TestMode() bool {
	return c.defined["test-mode"].(bool)
}

// LXCUseClone reports whether the LXC provisioner should create a
// template and use cloning to speed up container provisioning.
func (c *Config) LXCUseClone() (bool, bool) {
	v, ok := c.defined[LxcClone].(bool)
	return v, ok
}

// LXCUseCloneAUFS reports whether the LXC provisioner should create a
// lxc clone using aufs if available.
func (c *Config) LXCUseCloneAUFS() (bool, bool) {
	v, ok := c.defined["lxc-clone-aufs"].(bool)
	return v, ok
}

// LXCDefaultMTU reports whether the LXC provisioner should create a
// containers with a specific MTU value for all network intefaces.
func (c *Config) LXCDefaultMTU() (int, bool) {
	v, ok := c.defined[LXCDefaultMTU].(int)
	if !ok {
		return DefaultLXCDefaultMTU, false
	}
	return v, ok
}

// DisableNetworkManagement reports whether Juju is allowed to
// configure and manage networking inside the environment.
func (c *Config) DisableNetworkManagement() (bool, bool) {
	v, ok := c.defined["disable-network-management"].(bool)
	return v, ok
}

// IgnoreMachineAddresses reports whether Juju will discover
// and store machine addresses on startup.
func (c *Config) IgnoreMachineAddresses() (bool, bool) {
	v, ok := c.defined[IgnoreMachineAddresses].(bool)
	return v, ok
}

// StorageDefaultBlockSource returns the default block storage
// source for the environment.
func (c *Config) StorageDefaultBlockSource() (string, bool) {
	bs := c.asString(StorageDefaultBlockSourceKey)
	return bs, bs != ""
}

// AllowLXCLoopMounts returns whether loop devices are allowed
// to be mounted inside lxc containers.
func (c *Config) AllowLXCLoopMounts() (bool, bool) {
	v, ok := c.defined[AllowLXCLoopMounts].(bool)
	return v, ok
}

// ResourceTags returns a set of tags to set on environment resources
// that Juju creates and manages, if the provider supports them. These
// tags have no special meaning to Juju, but may be used for existing
// chargeback accounting schemes or other identification purposes.
func (c *Config) ResourceTags() (map[string]string, bool) {
	tags, err := c.resourceTags()
	if err != nil {
		panic(err) // should be prevented by Validate
	}
	return tags, tags != nil
}

func (c *Config) resourceTags() (map[string]string, error) {
	v, ok := c.defined[ResourceTagsKey].(map[string]string)
	if !ok {
		return nil, nil
	}
	for k := range v {
		if strings.HasPrefix(k, tags.JujuTagPrefix) {
			return nil, errors.Errorf("tag %q uses reserved prefix %q", k, tags.JujuTagPrefix)
		}
	}
	return v, nil
}

// UnknownAttrs returns a copy of the raw configuration attributes
// that are supposedly specific to the environment type. They could
// also be wrong attributes, though. Only the specific environment
// implementation can tell.
func (c *Config) UnknownAttrs() map[string]interface{} {
	newAttrs := make(map[string]interface{})
	for k, v := range c.unknown {
		newAttrs[k] = v
	}
	return newAttrs
}

// AllAttrs returns a copy of the raw configuration attributes.
func (c *Config) AllAttrs() map[string]interface{} {
	allAttrs := c.UnknownAttrs()
	for k, v := range c.defined {
		allAttrs[k] = v
	}
	return allAttrs
}

// Remove returns a new configuration that has the attributes of c minus attrs.
func (c *Config) Remove(attrs []string) (*Config, error) {
	defined := c.AllAttrs()
	for _, k := range attrs {
		delete(defined, k)
	}
	return New(NoDefaults, defined)
}

// Apply returns a new configuration that has the attributes of c plus attrs.
func (c *Config) Apply(attrs map[string]interface{}) (*Config, error) {
	defined := c.AllAttrs()
	for k, v := range attrs {
		defined[k] = v
	}
	return New(NoDefaults, defined)
}

// fields holds the validation schema fields derived from configSchema.
var fields = func() schema.Fields {
	fs, _, err := configSchema.ValidationSchema()
	if err != nil {
		panic(err)
	}
	return fs
}()

// alwaysOptional holds configuration defaults for attributes that may
// be unspecified even after a configuration has been created with all
// defaults filled out.
//
// This table is not definitive: it specifies those attributes which are
// optional when the config goes through its initial schema coercion,
// but some fields listed as optional here are actually mandatory
// with NoDefaults and are checked at the later Validate stage.
var alwaysOptional = schema.Defaults{
	"agent-version":              schema.Omit,
	"ca-cert":                    schema.Omit,
	"authorized-keys":            schema.Omit,
	"authorized-keys-path":       schema.Omit,
	"ca-cert-path":               schema.Omit,
	"ca-private-key-path":        schema.Omit,
	"logging-config":             schema.Omit,
	ProvisionerHarvestModeKey:    schema.Omit,
	"bootstrap-timeout":          schema.Omit,
	"bootstrap-retry-delay":      schema.Omit,
	"bootstrap-addresses-delay":  schema.Omit,
	"rsyslog-ca-cert":            schema.Omit,
	"rsyslog-ca-key":             schema.Omit,
	HttpProxyKey:                 schema.Omit,
	HttpsProxyKey:                schema.Omit,
	FtpProxyKey:                  schema.Omit,
	NoProxyKey:                   schema.Omit,
	AptHttpProxyKey:              schema.Omit,
	AptHttpsProxyKey:             schema.Omit,
	AptFtpProxyKey:               schema.Omit,
	"apt-mirror":                 schema.Omit,
	LxcClone:                     schema.Omit,
	LXCDefaultMTU:                schema.Omit,
	"disable-network-management": schema.Omit,
	IgnoreMachineAddresses:       schema.Omit,
	AgentStreamKey:               schema.Omit,
	SetNumaControlPolicyKey:      DefaultNumaControlPolicy,
	AllowLXCLoopMounts:           false,
	ResourceTagsKey:              schema.Omit,

	// Storage related config.
	// Environ providers will specify their own defaults.
	StorageDefaultBlockSourceKey: schema.Omit,

	// Deprecated fields, retain for backwards compatibility.
	ToolsMetadataURLKey:          "",
	LxcUseClone:                  schema.Omit,
	ProvisionerSafeModeKey:       schema.Omit,
	ToolsStreamKey:               schema.Omit,
	PreventDestroyEnvironmentKey: schema.Omit,
	PreventRemoveObjectKey:       schema.Omit,
	PreventAllChangesKey:         schema.Omit,

	// For backward compatibility reasons, the following
	// attributes default to empty strings rather than being
	// omitted.
	// TODO(rog) remove this support when we can
	// remove upgrade compatibility with versions prior to 1.14.
	"admin-secret":       "", // TODO(rog) omit
	"ca-private-key":     "", // TODO(rog) omit
	"image-metadata-url": "", // TODO(rog) omit
	AgentMetadataURLKey:  "", // TODO(rog) omit

	"default-series": "",

	// For backward compatibility only - default ports were
	// not filled out in previous versions of the configuration.
	"state-port":  DefaultStatePort,
	"api-port":    DefaultAPIPort,
	"syslog-port": DefaultSyslogPort,
	// Previously image-stream could be set to an empty value
	"image-stream":             "",
	"test-mode":                false,
	"proxy-ssh":                false,
	"lxc-clone-aufs":           false,
	"prefer-ipv6":              false,
	"enable-os-refresh-update": schema.Omit,
	"enable-os-upgrade":        schema.Omit,

	// uuid may be missing for backwards compatability.
	"uuid": schema.Omit,
}

func allowEmpty(attr string) bool {
	return alwaysOptional[attr] == ""
}

var defaults = allDefaults()

// allDefaults returns a schema.Defaults that contains
// defaults to be used when creating a new config with
// UseDefaults.
func allDefaults() schema.Defaults {
	d := schema.Defaults{
		"firewall-mode":              FwInstance,
		"development":                false,
		"ssl-hostname-verification":  true,
		"state-port":                 DefaultStatePort,
		"api-port":                   DefaultAPIPort,
		"syslog-port":                DefaultSyslogPort,
		"bootstrap-timeout":          DefaultBootstrapSSHTimeout,
		"bootstrap-retry-delay":      DefaultBootstrapSSHRetryDelay,
		"bootstrap-addresses-delay":  DefaultBootstrapSSHAddressesDelay,
		"proxy-ssh":                  true,
		"prefer-ipv6":                false,
		"disable-network-management": false,
		IgnoreMachineAddresses:       false,
		SetNumaControlPolicyKey:      DefaultNumaControlPolicy,
	}
	for attr, val := range alwaysOptional {
		if _, ok := d[attr]; !ok {
			d[attr] = val
		}
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
	"uuid",
	"firewall-mode",
	"state-port",
	"api-port",
	"bootstrap-timeout",
	"bootstrap-retry-delay",
	"bootstrap-addresses-delay",
	LxcClone,
	LXCDefaultMTU,
	"lxc-clone-aufs",
	"syslog-port",
	"prefer-ipv6",
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
		// TODO(ericsnow) Drop this?
		logger.Debugf("coercion failed attributes: %#v, checker: %#v, %v", attrs, checker, err)
		return nil, err
	}
	result := coerced.(map[string]interface{})
	for name, value := range attrs {
		if fields[name] == nil {
			if val, isString := value.(string); isString && val != "" {
				// only warn about attributes with non-empty string values
				logger.Warningf("unknown config field %q", name)
			}
			result[name] = value
		}
	}
	return result, nil
}

// GenerateStateServerCertAndKey makes sure that the config has a CACert and
// CAPrivateKey, generates and returns new certificate and key.
func (cfg *Config) GenerateStateServerCertAndKey(hostAddresses []string) (string, string, error) {
	caCert, hasCACert := cfg.CACert()
	if !hasCACert {
		return "", "", fmt.Errorf("environment configuration has no ca-cert")
	}
	caKey, hasCAKey := cfg.CAPrivateKey()
	if !hasCAKey {
		return "", "", fmt.Errorf("environment configuration has no ca-private-key")
	}
	return cert.NewDefaultServer(caCert, caKey, hostAddresses)
}

// SpecializeCharmRepo customizes a repository for a given configuration.
// It returns a charm repository with test mode enabled if applicable.
func SpecializeCharmRepo(repo charmrepo.Interface, cfg *Config) charmrepo.Interface {
	type specializer interface {
		WithTestMode() charmrepo.Interface
	}
	if store, ok := repo.(specializer); ok {
		if cfg.TestMode() {
			return store.WithTestMode()
		}
	}
	return repo
}

// SSHTimeoutOpts lists the amount of time we will wait for various
// parts of the SSH connection to complete. This is similar to
// DialOpts, see http://pad.lv/1258889 about possibly deduplicating
// them.
type SSHTimeoutOpts struct {
	// Timeout is the amount of time to wait contacting a state
	// server.
	Timeout time.Duration

	// RetryDelay is the amount of time between attempts to connect to
	// an address.
	RetryDelay time.Duration

	// AddressesDelay is the amount of time between refreshing the
	// addresses.
	AddressesDelay time.Duration
}

func addIfNotEmpty(settings map[string]interface{}, key, value string) {
	if value != "" {
		settings[key] = value
	}
}

// ProxyConfigMap returns a map suitable to be applied to a Config to update
// proxy settings.
func ProxyConfigMap(proxySettings proxy.Settings) map[string]interface{} {
	settings := make(map[string]interface{})
	addIfNotEmpty(settings, HttpProxyKey, proxySettings.Http)
	addIfNotEmpty(settings, HttpsProxyKey, proxySettings.Https)
	addIfNotEmpty(settings, FtpProxyKey, proxySettings.Ftp)
	addIfNotEmpty(settings, NoProxyKey, proxySettings.NoProxy)
	return settings
}

// AptProxyConfigMap returns a map suitable to be applied to a Config to update
// proxy settings.
func AptProxyConfigMap(proxySettings proxy.Settings) map[string]interface{} {
	settings := make(map[string]interface{})
	addIfNotEmpty(settings, AptHttpProxyKey, proxySettings.Http)
	addIfNotEmpty(settings, AptHttpsProxyKey, proxySettings.Https)
	addIfNotEmpty(settings, AptFtpProxyKey, proxySettings.Ftp)
	return settings
}

// Schema returns a configuration schema that includes both
// the given extra fields and all the fields defined in this package.
// It returns an error if extra defines any fields defined in this
// package.
func Schema(extra environschema.Fields) (environschema.Fields, error) {
	fields := make(environschema.Fields)
	for name, field := range configSchema {
		fields[name] = field
	}
	for name, field := range extra {
		if _, ok := fields[name]; ok {
			return nil, errors.Errorf("config field %q clashes with global config", name)
		}
		fields[name] = field
	}
	return fields, nil
}

// configSchema holds information on all the fields defined by
// the config package.
// TODO(rog) make this available to external packages.
var configSchema = environschema.Fields{
	"admin-secret": {
		Description: "The password for the administrator user",
		Type:        environschema.Tstring,
		Secret:      true,
		Example:     "<random secret>",
		Group:       environschema.EnvironGroup,
	},
	AgentMetadataURLKey: {
		Description: "URL of private stream",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	AgentStreamKey: {
		Description: `Version of Juju to use for deploy/upgrades.`,
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	"agent-version": {
		Description: "The desired Juju agent version to use",
		Type:        environschema.Tstring,
		Group:       environschema.JujuGroup,
		Immutable:   true,
	},
	AllowLXCLoopMounts: {
		Description: `whether loop devices are allowed to be mounted inside lxc containers.`,
		Type:        environschema.Tbool,
		Group:       environschema.EnvironGroup,
	},
	"api-port": {
		Description: "The TCP port for the API servers to listen on",
		Type:        environschema.Tint,
		Group:       environschema.EnvironGroup,
		Immutable:   true,
	},
	AptFtpProxyKey: {
		// TODO document acceptable format
		Description: "The APT FTP proxy for the environment",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	AptHttpProxyKey: {
		// TODO document acceptable format
		Description: "The APT HTTP proxy for the environment",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	AptHttpsProxyKey: {
		// TODO document acceptable format
		Description: "The APT HTTPS proxy for the environment",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	"apt-mirror": {
		// TODO document acceptable format
		Description: "The APT mirror for the environment",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	"authorized-keys": {
		// TODO what to do about authorized-keys-path ?
		Description: "Any authorized SSH public keys for the environment, as found in a ~/.ssh/authorized_keys file",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	"authorized-keys-path": {
		Description: "Path to file containing SSH authorized keys",
		Type:        environschema.Tstring,
	},
	PreventAllChangesKey: {
		Description: `Whether all changes to the environment will be prevented`,
		Type:        environschema.Tbool,
		Group:       environschema.EnvironGroup,
	},
	PreventDestroyEnvironmentKey: {
		Description: `Whether the environment will be prevented from destruction`,
		Type:        environschema.Tbool,
		Group:       environschema.EnvironGroup,
	},
	PreventRemoveObjectKey: {
		Description: `Whether remove operations (machine, service, unit or relation) will be prevented`,
		Type:        environschema.Tbool,
		Group:       environschema.EnvironGroup,
	},
	"bootstrap-addresses-delay": {
		Description: "The amount of time between refreshing the addresses in seconds. Not too frequent as we refresh addresses from the provider each time.",
		Type:        environschema.Tint,
		Immutable:   true,
		Group:       environschema.EnvironGroup,
	},
	"bootstrap-retry-delay": {
		Description: "Time between attempts to connect to an address in seconds.",
		Type:        environschema.Tint,
		Immutable:   true,
		Group:       environschema.EnvironGroup,
	},
	"bootstrap-timeout": {
		Description: "The amount of time to wait contacting a state server in seconds",
		Type:        environschema.Tint,
		Immutable:   true,
		Group:       environschema.EnvironGroup,
	},
	"ca-cert": {
		Description: `The certificate of the CA that signed the state server certificate, in PEM format`,
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	"ca-cert-path": {
		Description: "Path to file containing CA certificate",
		Type:        environschema.Tstring,
	},
	"ca-private-key": {
		Description: `The private key of the CA that signed the state server certificate, in PEM format`,
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	"ca-private-key-path": {
		Description: "Path to file containing CA private key",
		Type:        environschema.Tstring,
	},
	"default-series": {
		Description: "The default series of Ubuntu to use for deploying charms",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	"development": {
		Description: "Whether the environment is in development mode",
		Type:        environschema.Tbool,
		Group:       environschema.EnvironGroup,
	},
	"disable-network-management": {
		Description: "Whether the provider should control networks (on MAAS environments, set to true for MAAS to control networks",
		Type:        environschema.Tbool,
		Group:       environschema.EnvironGroup,
	},
	IgnoreMachineAddresses: {
		Description: "Whether the machine worker should discover machine addresses on startup",
		Type:        environschema.Tbool,
		Group:       environschema.EnvironGroup,
	},
	"enable-os-refresh-update": {
		Description: `Whether newly provisioned instances should run their respective OS's update capability.`,
		Type:        environschema.Tbool,
		Group:       environschema.EnvironGroup,
	},
	"enable-os-upgrade": {
		Description: `Whether newly provisioned instances should run their respective OS's upgrade capability.`,
		Type:        environschema.Tbool,
		Group:       environschema.EnvironGroup,
	},
	"firewall-mode": {
		Description: `The mode to use for network firewalling.

'instance' requests the use of an individual firewall per instance.

'global' uses a single firewall for all instances (access
for a network port is enabled to one instance if any instance requires
that port).

'none' requests that no firewalling should be performed
inside the environment. It's useful for clouds without support for either
global or per instance security groups.`,
		Type: environschema.Tstring,
		// Note that we need the empty value because it can
		// be found in legacy environments.
		Values:    []interface{}{FwInstance, FwGlobal, FwNone, ""},
		Immutable: true,
		Group:     environschema.EnvironGroup,
	},
	FtpProxyKey: {
		Description: "The FTP proxy value to configure on instances, in the FTP_PROXY environment variable",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	HttpProxyKey: {
		Description: "The HTTP proxy value to configure on instances, in the HTTP_PROXY environment variable",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	HttpsProxyKey: {
		Description: "The HTTPS proxy value to configure on instances, in the HTTPS_PROXY environment variable",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	"image-metadata-url": {
		Description: "The URL at which the metadata used to locate OS image ids is located",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	"image-stream": {
		Description: `The simplestreams stream used to identify which image ids to search when starting an instance.`,
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	"logging-config": {
		Description: `The configuration string to use when configuring Juju agent logging (see http://godoc.org/github.com/juju/loggo#ParseConfigurationString for details)`,
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	LxcClone: {
		Description: "Whether to use lxc-clone to create new LXC containers",
		Type:        environschema.Tbool,
		Immutable:   true,
		Group:       environschema.EnvironGroup,
	},
	"lxc-clone-aufs": {
		Description: `Whether the LXC provisioner should creat an LXC clone using AUFS if available`,
		Type:        environschema.Tbool,
		Immutable:   true,
		Group:       environschema.EnvironGroup,
	},
	LXCDefaultMTU: {
		// default: the default MTU setting for the container
		Description: `The MTU setting to use for network interfaces in LXC containers`,
		Type:        environschema.Tint,
		Immutable:   true,
		Group:       environschema.EnvironGroup,
	},
	LxcUseClone: {
		Description: `Whether the LXC provisioner should create a template and use cloning to speed up container provisioning. (deprecated by lxc-clone)`,
		Type:        environschema.Tbool,
	},
	"name": {
		Description: "The name of the current environment",
		Type:        environschema.Tstring,
		Mandatory:   true,
		Immutable:   true,
		Group:       environschema.EnvironGroup,
	},
	NoProxyKey: {
		Description: "List of domain addresses not to be proxied (comma-separated)",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	"prefer-ipv6": {
		Description: `Whether to prefer IPv6 over IPv4 addresses for API endpoints and machines`,
		Type:        environschema.Tbool,
		Immutable:   true,
		Group:       environschema.EnvironGroup,
	},
	ProvisionerHarvestModeKey: {
		// default: destroyed, but also depends on current setting of ProvisionerSafeModeKey
		Description: "What to do with unknown machines. See https://jujucharms.com/docs/stable/config-general#juju-lifecycle-and-harvesting (default destroyed)",
		Type:        environschema.Tstring,
		Values:      []interface{}{"all", "none", "unknown", "destroyed"},
		Group:       environschema.EnvironGroup,
	},
	ProvisionerSafeModeKey: {
		Description: `Whether to run the provisioner in "destroyed" harvest mode (deprecated, superceded by provisioner-harvest-mode)`,
		Type:        environschema.Tbool,
		Group:       environschema.EnvironGroup,
	},
	"proxy-ssh": {
		// default: true
		Description: `Whether SSH commands should be proxied through the API server`,
		Type:        environschema.Tbool,
		Group:       environschema.EnvironGroup,
	},
	ResourceTagsKey: {
		Description: "resource tags",
		Type:        environschema.Tattrs,
		Group:       environschema.EnvironGroup,
	},
	"rsyslog-ca-cert": {
		Description: `The certificate of the CA that signed the rsyslog certificate, in PEM format.`,
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	"rsyslog-ca-key": {
		Description: `The private key of the CA that signed the rsyslog certificate, in PEM format`,
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	SetNumaControlPolicyKey: {
		Description: "Tune Juju state-server to work with NUMA if present (default false)",
		Type:        environschema.Tbool,
		Group:       environschema.EnvironGroup,
	},
	"ssl-hostname-verification": {
		Description: "Whether SSL hostname verification is enabled (default true)",
		Type:        environschema.Tbool,
		Group:       environschema.EnvironGroup,
	},
	StorageDefaultBlockSourceKey: {
		Description: "The default block storage source for the environment",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	"state-port": {
		Description: "Port for the API server to listen on.",
		Type:        environschema.Tint,
		Immutable:   true,
		Group:       environschema.EnvironGroup,
	},
	"syslog-port": {
		Description: "Port for the syslog UDP/TCP listener to listen on.",
		Type:        environschema.Tint,
		Immutable:   true,
		Group:       environschema.EnvironGroup,
	},
	"test-mode": {
		Description: `Whether the environment is intended for testing.
If true, accessing the charm store does not affect statistical
data of the store. (default false)`,
		Type:  environschema.Tbool,
		Group: environschema.EnvironGroup,
	},
	ToolsMetadataURLKey: {
		Description: `deprecated, superceded by agent-metadata-url`,
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	ToolsStreamKey: {
		Description: `deprecated, superceded by agent-stream`,
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	"type": {
		Description: "Type of environment, e.g. local, ec2",
		Type:        environschema.Tstring,
		Mandatory:   true,
		Immutable:   true,
		Group:       environschema.EnvironGroup,
	},
	"uuid": {
		Description: "The UUID of the environment",
		Type:        environschema.Tstring,
		Group:       environschema.JujuGroup,
		Immutable:   true,
	},
}
