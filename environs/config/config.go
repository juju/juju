// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/featureflag"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	"github.com/juju/proxy"
	"github.com/juju/schema"
	"github.com/juju/utils/v3"
	"github.com/juju/version/v2"
	"gopkg.in/juju/environschema.v1"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/charmhub"
	"github.com/juju/juju/controller"
	corebase "github.com/juju/juju/core/base"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/logfwd/syslog"
	jujuversion "github.com/juju/juju/version"
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
)

// TODO(katco-): Please grow this over time.
// Centralized place to store values of config keys. This transitions
// mistakes in referencing key-values to a compile-time error.
const (
	//
	// Settings Attributes
	//

	// NameKey is the key for the model's name.
	NameKey = "name"

	// TypeKey is the key for the model's cloud type.
	TypeKey = "type"

	// AgentVersionKey is the key for the model's Juju agent version.
	AgentVersionKey = "agent-version"

	// UUIDKey is the key for the model UUID attribute.
	UUIDKey = "uuid"

	// AuthorizedKeysKey is the key for the authorized-keys attribute.
	AuthorizedKeysKey = "authorized-keys"

	// ProvisionerHarvestModeKey stores the key for this setting.
	ProvisionerHarvestModeKey = "provisioner-harvest-mode"

	// NumProvisionWorkersKey is the key for number of model provisioner
	// workers.
	NumProvisionWorkersKey = "num-provision-workers"

	// NumContainerProvisionWorkersKey is the key for the number of
	// container provisioner workers per machine setting.
	NumContainerProvisionWorkersKey = "num-container-provision-workers"

	// ImageStreamKey is the key used to specify the stream
	// for OS images.
	ImageStreamKey = "image-stream"

	// ImageMetadataURLKey is the key used to specify the location
	// of OS image metadata.
	ImageMetadataURLKey = "image-metadata-url"

	// ImageMetadataDefaultsDisabledKey is the key used to disable image
	// metadata default sources.
	ImageMetadataDefaultsDisabledKey = "image-metadata-defaults-disabled"

	// AgentStreamKey stores the key for this setting.
	AgentStreamKey = "agent-stream"

	// AgentMetadataURLKey stores the key for this setting.
	AgentMetadataURLKey = "agent-metadata-url"

	// ContainerImageStreamKey is the key used to specify the stream
	// for container OS images.
	ContainerImageStreamKey = "container-image-stream"

	// ContainerImageMetadataURLKey is the key used to specify the location
	// of OS image metadata for containers.
	ContainerImageMetadataURLKey = "container-image-metadata-url"

	// ContainerImageMetadataDefaultsDisabledKey is the key used to disable image
	// metadata default sources for containers.
	ContainerImageMetadataDefaultsDisabledKey = "container-image-metadata-defaults-disabled"

	// Proxy behaviour has become something of an annoying thing to define
	// well. These following four proxy variables are being kept to continue
	// with the existing behaviour for those deployments that specify them.
	// With these proxy values set, a file is written to every machine
	// in /etc/profile.d so the ubuntu user gets the environment variables
	// set when SSHing in. The OS environment also is set in the juju agents
	// and charm hook environments.

	// HTTPProxyKey stores the key for this setting.
	HTTPProxyKey = "http-proxy"

	// HTTPSProxyKey stores the key for this setting.
	HTTPSProxyKey = "https-proxy"

	// FTPProxyKey stores the key for this setting.
	FTPProxyKey = "ftp-proxy"

	// NoProxyKey stores the key for this setting.
	NoProxyKey = "no-proxy"

	// The new proxy keys are passed into hook contexts with the prefix
	// JUJU_CHARM_ then HTTP_PROXY, HTTPS_PROXY, FTP_PROXY, and NO_PROXY.
	// This allows the charm to set a proxy when it thinks it needs one.
	// These values are not set in the general environment.

	// JujuHTTPProxyKey stores the key for this setting.
	JujuHTTPProxyKey = "juju-http-proxy"

	// JujuHTTPSProxyKey stores the key for this setting.
	JujuHTTPSProxyKey = "juju-https-proxy"

	// JujuFTPProxyKey stores the key for this setting.
	JujuFTPProxyKey = "juju-ftp-proxy"

	// JujuNoProxyKey stores the key for this setting.
	JujuNoProxyKey = "juju-no-proxy"

	// The APT proxy values specified here work with both the
	// legacy and juju proxy settings. If no value is specified,
	// the value is determined by the either the legacy or juju value
	// if one is specified.

	// AptHTTPProxyKey stores the key for this setting.
	AptHTTPProxyKey = "apt-http-proxy"

	// AptHTTPSProxyKey stores the key for this setting.
	AptHTTPSProxyKey = "apt-https-proxy"

	// AptFTPProxyKey stores the key for this setting.
	AptFTPProxyKey = "apt-ftp-proxy"

	// AptNoProxyKey stores the key for this setting.
	AptNoProxyKey = "apt-no-proxy"

	// AptMirrorKey is used to set the apt mirror.
	AptMirrorKey = "apt-mirror"

	// SnapHTTPProxyKey is used to set the snap core setting proxy.http for deployed machines.
	SnapHTTPProxyKey = "snap-http-proxy"
	// SnapHTTPSProxyKey is used to set the snap core setting proxy.https for deployed machines.
	SnapHTTPSProxyKey = "snap-https-proxy"
	// SnapStoreProxyKey is used to set the snap core setting proxy.store for deployed machines.
	SnapStoreProxyKey = "snap-store-proxy"
	// SnapStoreAssertionsKey is used to configure the deployed machines to acknowledge the
	// store proxy assertions.
	SnapStoreAssertionsKey = "snap-store-assertions"
	// SnapStoreProxyURLKey is used to specify the URL to a snap store proxy.
	SnapStoreProxyURLKey = "snap-store-proxy-url"

	// NetBondReconfigureDelayKey is the key to pass when bridging
	// the network for containers.
	NetBondReconfigureDelayKey = "net-bond-reconfigure-delay"

	// ContainerNetworkingMethod is the key for setting up
	// networking method for containers.
	ContainerNetworkingMethod = "container-networking-method"

	// StorageDefaultBlockSourceKey is the key for the default block storage source.
	StorageDefaultBlockSourceKey = "storage-default-block-source"

	// StorageDefaultFilesystemSourceKey is the key for the default filesystem storage source.
	StorageDefaultFilesystemSourceKey = "storage-default-filesystem-source"

	// ResourceTagsKey is an optional list or space-separated string
	// of k=v pairs, defining the tags for ResourceTags.
	ResourceTagsKey = "resource-tags"

	// LogForwardEnabled determines whether the log forward functionality is enabled.
	LogForwardEnabled = "logforward-enabled"

	// LogFwdSyslogHost sets the hostname:port of the syslog server.
	LogFwdSyslogHost = "syslog-host"

	// LogFwdSyslogCACert sets the certificate of the CA that signed the syslog
	// server certificate.
	LogFwdSyslogCACert = "syslog-ca-cert"

	// LogFwdSyslogClientCert sets the client certificate for syslog
	// forwarding.
	LogFwdSyslogClientCert = "syslog-client-cert"

	// LogFwdSyslogClientKey sets the client key for syslog
	// forwarding.
	LogFwdSyslogClientKey = "syslog-client-key"

	// AutomaticallyRetryHooks determines whether the uniter will
	// automatically retry a hook that has failed
	AutomaticallyRetryHooks = "automatically-retry-hooks"

	// TransmitVendorMetricsKey is the key for whether the controller sends
	// metrics collected in this model for anonymized aggregate analytics.
	TransmitVendorMetricsKey = "transmit-vendor-metrics"

	// ExtraInfoKey is the key for arbitrary user specified string data that
	// is stored against the model.
	ExtraInfoKey = "extra-info"

	// MaxStatusHistoryAge is the maximum age of status history values
	// to keep when pruning, eg "72h"
	MaxStatusHistoryAge = "max-status-history-age"

	// MaxStatusHistorySize is the maximum size the status history
	// collection can grow to before it is pruned, eg "5M"
	MaxStatusHistorySize = "max-status-history-size"

	// MaxActionResultsAge is the maximum age of actions to keep when pruning, eg
	// "72h"
	MaxActionResultsAge = "max-action-results-age"

	// MaxActionResultsSize is the maximum size the actions collection can
	// grow to before it is pruned, eg "5M"
	MaxActionResultsSize = "max-action-results-size"

	// UpdateStatusHookInterval is how often to run the update-status hook.
	UpdateStatusHookInterval = "update-status-hook-interval"

	// EgressSubnets are the source addresses from which traffic from this model
	// originates if the model is deployed such that NAT or similar is in use.
	EgressSubnets = "egress-subnets"

	// FanConfig defines the configuration for FAN network running in the model.
	FanConfig = "fan-config"

	// CloudInitUserDataKey is the key to specify cloud-init yaml the user
	// wants to add into the cloud-config data produced by Juju when
	// provisioning machines.
	CloudInitUserDataKey = "cloudinit-userdata"

	// BackupDirKey specifies the backup working directory.
	BackupDirKey = "backup-dir"

	// ContainerInheritPropertiesKey is the key to specify a list of properties
	// to be copied from a machine to a container during provisioning. The
	// list will be comma separated.
	ContainerInheritPropertiesKey = "container-inherit-properties"

	// DefaultSpace specifies which space should be used for the default
	// endpoint bindings.
	DefaultSpace = "default-space"

	// LXDSnapChannel selects the channel to use when installing LXD from a snap.
	LXDSnapChannel = "lxd-snap-channel"

	// CharmHubURLKey is the key for the url to use for CharmHub API calls
	CharmHubURLKey = "charmhub-url"

	// ModeKey is the key for defining the mode that a given model should be
	// using.
	// It is expected that when in a different mode, Juju will perform in a
	// different state.
	ModeKey = "mode"

	// SSHAllowKey is a comma separated list of CIDRs from which machines in
	// this model will accept connections to the SSH service
	SSHAllowKey = "ssh-allow"

	// SAASIngressAllowKey is a comma separated list of CIDRs
	// specifying what ingress can be applied to offers in this model
	SAASIngressAllowKey = "saas-ingress-allow"

	//
	// Deprecated Settings Attributes
	//

	// IgnoreMachineAddresses when true, will cause the
	// machine worker not to discover any machine addresses
	// on start up.
	IgnoreMachineAddresses = "ignore-machine-addresses"

	// TestModeKey is the key for identifying the model should be run in test
	// mode.
	TestModeKey = "test-mode"

	// DisableTelemetryKey is a key for determining whether telemetry on juju
	// models will be done.
	DisableTelemetryKey = "disable-telemetry"

	// LoggingOutputKey is a key for determining the destination of output for
	// logging.
	LoggingOutputKey = "logging-output"

	// DefaultSeriesKey is a key for determining the series a model should
	// explicitly use for charms unless otherwise provided.
	DefaultSeriesKey = "default-series"

	// DefaultBaseKey is a key for determining the base a model should
	// explicitly use for charms unless otherwise provided.
	DefaultBaseKey = "default-base"

	// SecretBackendKey is used to specify the secret backend.
	SecretBackendKey = "secret-backend"

	// ContainerImageReference is used to specify the container image repository.
	ContainerImageReference = "container-image-reference"
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
	// through a destroy of an application/model/unit.
	HarvestDestroyed
	// HarvestAll signifies that Juju should harvest both unknown and
	// destroyed instances. ♫ Don't fear the reaper. ♫
	HarvestAll = HarvestUnknown | HarvestDestroyed
)

// A mapping from method to description. Going this way will be the
// more common operation, so we want this type of lookup to be O(1).
var harvestingMethodToFlag = map[HarvestMode]string{
	HarvestAll:       "all",
	HarvestNone:      "none",
	HarvestUnknown:   "unknown",
	HarvestDestroyed: "destroyed",
}

// String returns the description of the harvesting mode.
func (method HarvestMode) String() string {
	if description, ok := harvestingMethodToFlag[method]; ok {
		return description
	}
	panic("Unknown harvesting method.")
}

// HarvestNone returns whether or not the None harvesting flag is set.
func (method HarvestMode) HarvestNone() bool {
	return method&HarvestNone != 0
}

// HarvestDestroyed returns whether or not the Destroyed harvesting flag is set.
func (method HarvestMode) HarvestDestroyed() bool {
	return method&HarvestDestroyed != 0
}

// HarvestUnknown returns whether or not the Unknown harvesting flag is set.
func (method HarvestMode) HarvestUnknown() bool {
	return method&HarvestUnknown != 0
}

// GetDefaultSupportedLTSBase returns the DefaultSupportedLTSBase.
// This is exposed for one reason and one reason only; testing!
// The fact that PreferredBase doesn't take an argument for a default base
// as a fallback. We then have to expose this so we can exercise the branching
// code for other scenarios makes me sad.
var GetDefaultSupportedLTSBase = jujuversion.DefaultSupportedLTSBase

// HasDefaultBase defines a interface if a type has a default base or not.
type HasDefaultBase interface {
	DefaultBase() (string, bool)
}

// PreferredBase returns the preferred base to use when a charm does not
// explicitly specify a base.
func PreferredBase(cfg HasDefaultBase) corebase.Base {
	base, ok := cfg.DefaultBase()
	if ok {
		// We can safely ignore the error here as we know that we have
		// validated the base when we set it.
		return corebase.MustParseBaseFromString(base)
	}
	return GetDefaultSupportedLTSBase()
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
	// UseDefaults defines a constant for indicating of the default should be
	// used for the configuration.
	UseDefaults Defaulting = true
	// NoDefaults defines a constant for indicating that no defaults should be
	// used for the configuration.
	NoDefaults Defaulting = false
)

// TODO(rog) update the doc comment below - it's getting messy
// and it assumes too much prior knowledge.

// New returns a new configuration.  Fields that are common to all
// environment providers are verified.  If useDefaults is UseDefaults,
// default values will be taken from the environment.
//
// "ca-cert-path" and "ca-private-key-path" are translated into the
// "ca-cert" and "ca-private-key" values.  If not specified, CA details
// will be read from:
//
//	~/.local/share/juju/<name>-cert.pem
//	~/.local/share/juju/<name>-private-key.pem
//
// if $XDG_DATA_HOME is defined it will be used instead of ~/.local/share
//
// The attrs map can not be nil, otherwise a panic is raised.
func New(withDefaults Defaulting, attrs map[string]interface{}) (*Config, error) {
	ensureSchema()

	checker := noDefaultsChecker
	if withDefaults {
		checker = withDefaultsChecker
	} else {
		// Config may be from an older Juju.
		// Handle the case where we are parsing a fully formed
		// set of config attributes (NoDefaults) and a value is strictly
		// not optional, but may have previously been either set to empty
		// or is missing.
		// In this case, we use the default.
		for k := range defaultsWhenParsing {
			v, ok := attrs[k]
			if ok && v != "" {
				continue
			}
			_, explicitlyOptional := alwaysOptional[k]
			if !explicitlyOptional {
				attrs[k] = defaultsWhenParsing[k]
			}
		}
	}

	defined, err := checker.Coerce(attrs, nil)
	if err != nil {
		return nil, errors.Trace(err)
	}

	c := &Config{
		defined: defined.(map[string]interface{}),
		unknown: make(map[string]interface{}),
	}
	if err := c.setLoggingFromEnviron(); err != nil {
		return nil, errors.Trace(err)
	}

	// no old config to compare against
	if err := Validate(c, nil); err != nil {
		return nil, errors.Trace(err)
	}
	// Copy unknown attributes onto the type-specific map.
	for k, v := range attrs {
		if _, ok := allFields[k]; !ok {
			c.unknown[k] = v
		}
	}
	return c, nil
}

const (
	// DefaultStatusHistoryAge is the default value for MaxStatusHistoryAge.
	DefaultStatusHistoryAge = "336h" // 2 weeks

	// DefaultStatusHistorySize is the default value for MaxStatusHistorySize.
	DefaultStatusHistorySize = "5G"

	// DefaultUpdateStatusHookInterval is the default value for
	// UpdateStatusHookInterval
	DefaultUpdateStatusHookInterval = "5m"

	// DefaultActionResultsAge is the default for the age of the results for an
	// action.
	DefaultActionResultsAge = "336h" // 2 weeks

	// DefaultActionResultsSize is the default size of the action results.
	DefaultActionResultsSize = "5G"

	// DefaultLxdSnapChannel is the default lxd snap channel to install on host vms.
	DefaultLxdSnapChannel = "5.0/stable"

	// DefaultSecretBackend is the default secret backend to use.
	DefaultSecretBackend = "auto"
)

var defaultConfigValues = map[string]interface{}{
	// Network.
	"firewall-mode":              FwInstance,
	"disable-network-management": false,
	IgnoreMachineAddresses:       false,
	"ssl-hostname-verification":  true,
	"proxy-ssh":                  false,
	DefaultSpace:                 "",
	// Why is net-bond-reconfigure-delay set to 17 seconds?
	//
	// The value represents the amount of time in seconds to sleep
	// between ifdown and ifup when bridging bonded interfaces;
	// this is a platform bug and all of this can go away when bug
	// #1657579 (and #1594855 and #1269921) are fixed.
	//
	// For a long time the bridge script hardcoded a value of 3s
	// but some setups now require an even longer period. The last
	// reported issue was fixed with a 10s timeout, however, we're
	// increasing that because this issue (and solution) is not
	// very discoverable and we would like bridging to work
	// out-of-the-box.
	//
	// This value can be further tweaked via:
	//
	// $ juju model-config net-bond-reconfigure-delay=30
	NetBondReconfigureDelayKey: 17,
	ContainerNetworkingMethod:  "",

	DefaultBaseKey: "",

	ProvisionerHarvestModeKey:       HarvestDestroyed.String(),
	NumProvisionWorkersKey:          16,
	NumContainerProvisionWorkersKey: 4,
	ResourceTagsKey:                 "",
	"logging-config":                "",
	LoggingOutputKey:                "",
	AutomaticallyRetryHooks:         true,
	"enable-os-refresh-update":      true,
	"enable-os-upgrade":             true,
	"development":                   false,
	TestModeKey:                     false,
	ModeKey:                         RequiresPromptsMode,
	DisableTelemetryKey:             false,
	TransmitVendorMetricsKey:        true,
	UpdateStatusHookInterval:        DefaultUpdateStatusHookInterval,
	EgressSubnets:                   "",
	FanConfig:                       "",
	CloudInitUserDataKey:            "",
	ContainerInheritPropertiesKey:   "",
	BackupDirKey:                    "",
	LXDSnapChannel:                  DefaultLxdSnapChannel,

	CharmHubURLKey: charmhub.DefaultServerURL,

	// Image and agent streams and URLs.
	ImageStreamKey:                            "released",
	ImageMetadataURLKey:                       "",
	ImageMetadataDefaultsDisabledKey:          false,
	AgentStreamKey:                            "released",
	AgentMetadataURLKey:                       "",
	ContainerImageStreamKey:                   "released",
	ContainerImageMetadataURLKey:              "",
	ContainerImageMetadataDefaultsDisabledKey: false,
	ContainerImageReference:                   "",

	// Log forward settings.
	LogForwardEnabled: false,

	// Proxy settings.
	HTTPProxyKey:      "",
	HTTPSProxyKey:     "",
	FTPProxyKey:       "",
	NoProxyKey:        "127.0.0.1,localhost,::1",
	JujuHTTPProxyKey:  "",
	JujuHTTPSProxyKey: "",
	JujuFTPProxyKey:   "",
	JujuNoProxyKey:    "127.0.0.1,localhost,::1",

	AptHTTPProxyKey:  "",
	AptHTTPSProxyKey: "",
	AptFTPProxyKey:   "",
	AptNoProxyKey:    "",
	AptMirrorKey:     "",

	SnapHTTPProxyKey:       "",
	SnapHTTPSProxyKey:      "",
	SnapStoreProxyKey:      "",
	SnapStoreAssertionsKey: "",
	SnapStoreProxyURLKey:   "",

	// Status history settings
	MaxStatusHistoryAge:  DefaultStatusHistoryAge,
	MaxStatusHistorySize: DefaultStatusHistorySize,
	MaxActionResultsAge:  DefaultActionResultsAge,
	MaxActionResultsSize: DefaultActionResultsSize,

	// Secret settings.
	SecretBackendKey: DefaultSecretBackend,

	// Model firewall settings
	SSHAllowKey:         "0.0.0.0/0,::/0",
	SAASIngressAllowKey: "0.0.0.0/0,::/0",
}

// defaultLoggingConfig is the default value for logging-config if it is otherwise not set.
// We don't use the defaultConfigValues mechanism because one way to set the logging config is
// via the JUJU_LOGGING_CONFIG environment variable, which needs to be taken into account before
// we set the default.
const defaultLoggingConfig = "<root>=INFO"

// ConfigDefaults returns the config default values
// to be used for any new model where there is no
// value yet defined.
func ConfigDefaults() map[string]interface{} {
	defaults := make(map[string]interface{})
	for name, value := range defaultConfigValues {
		if developerConfigValue(name) {
			continue
		}
		defaults[name] = value
	}
	return defaults
}

func (c *Config) setLoggingFromEnviron() error {
	loggingConfig := c.asString("logging-config")
	// If the logging config hasn't been set, then look for the os environment
	// variable, and failing that, get the config from loggo itself.
	if loggingConfig == "" {
		if environmentValue := os.Getenv(osenv.JujuLoggingConfigEnvKey); environmentValue != "" {
			c.defined["logging-config"] = environmentValue
		} else {
			c.defined["logging-config"] = defaultLoggingConfig
		}
	}
	return nil
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
	// No deprecated attributes at the moment.
	return processedAttrs
}

// CoerceForStorage transforms attributes prior to being saved in a persistent store.
func CoerceForStorage(attrs map[string]any) (map[string]any, error) {
	ensureSchema()

	coercedAttrs := make(map[string]any, len(attrs))
	// Only coerce the fields we have values for.
	for k, v := range attrs {
		checker, ok := allFields[k]
		if !ok {
			// Preserve unknown attributes.
			coercedAttrs[k] = v
			continue
		}
		coercedValue, err := checker.Coerce(v, nil)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", k, err)
		}
		coercedAttrs[k] = coercedValue
	}

	if v, ok := coercedAttrs[ResourceTagsKey]; ok {
		// Resource Tags are specified by the user as a string but transformed
		// to a map when config is parsed. We want to store as a string.
		if tags, ok := v.(map[string]string); ok {
			var tagsSlice []string
			for resKey, resValue := range tags {
				tagsSlice = append(tagsSlice, fmt.Sprintf("%v=%v", resKey, resValue))
			}
			coercedAttrs[ResourceTagsKey] = strings.Join(tagsSlice, " ")
		}
	}

	return coercedAttrs, nil
}

// Validate ensures that config is a valid configuration.  If old is not nil,
// it holds the previous environment configuration for consideration when
// validating changes.
func Validate(cfg, old *Config) error {
	// Check that all other fields that have been specified are non-empty,
	// unless they're allowed to be empty for backward compatibility,
	for attr, val := range cfg.defined {
		if !isEmpty(val) {
			continue
		}
		if !allowEmpty(attr) {
			return fmt.Errorf("empty %s in model configuration", attr)
		}
	}

	modelName := cfg.asString(NameKey)
	if modelName == "" {
		return errors.New("empty name in model configuration")
	}
	if !names.IsValidModelName(modelName) {
		return fmt.Errorf("%q is not a valid name: model names may only contain lowercase letters, digits and hyphens", modelName)
	}

	// Check that the agent version parses ok if set explicitly; otherwise leave
	// it alone.
	if v, ok := cfg.defined[AgentVersionKey].(string); ok {
		if _, err := version.Parse(v); err != nil {
			return fmt.Errorf("invalid agent version in model configuration: %q", v)
		}
	}

	// If the logging config is set, make sure it is valid.
	if v, ok := cfg.defined["logging-config"].(string); ok {
		if _, err := loggo.ParseConfigString(v); err != nil {
			return err
		}
	}

	if lfCfg, ok := cfg.LogFwdSyslog(); ok {
		if err := lfCfg.Validate(); err != nil {
			return errors.Annotate(err, "invalid syslog forwarding config")
		}
	}

	if uuid := cfg.UUID(); !utils.IsValidUUIDString(uuid) {
		return errors.Errorf("uuid: expected UUID, got string(%q)", uuid)
	}

	// Ensure the resource tags have the expected k=v format.
	if _, err := cfg.resourceTags(); err != nil {
		return errors.Annotate(err, "validating resource tags")
	}

	if v, ok := cfg.defined[MaxStatusHistoryAge].(string); ok {
		if _, err := time.ParseDuration(v); err != nil {
			return errors.Annotate(err, "invalid max status history age in model configuration")
		}
	}

	if v, ok := cfg.defined[MaxStatusHistorySize].(string); ok {
		if _, err := utils.ParseSize(v); err != nil {
			return errors.Annotate(err, "invalid max status history size in model configuration")
		}
	}

	if v, ok := cfg.defined[MaxActionResultsAge].(string); ok {
		if _, err := time.ParseDuration(v); err != nil {
			return errors.Annotate(err, "invalid max action age in model configuration")
		}
	}

	if v, ok := cfg.defined[MaxActionResultsSize].(string); ok {
		if _, err := utils.ParseSize(v); err != nil {
			return errors.Annotate(err, "invalid max action size in model configuration")
		}
	}

	if v, ok := cfg.defined[UpdateStatusHookInterval].(string); ok {
		duration, err := time.ParseDuration(v)
		if err != nil {
			return errors.Annotate(err, "invalid update status hook interval in model configuration")
		}
		if duration < 1*time.Minute {
			return errors.Annotatef(err, "update status hook frequency %v cannot be less than 1m", duration)
		}
		if duration > 60*time.Minute {
			return errors.Annotatef(err, "update status hook frequency %v cannot be greater than 60m", duration)
		}
	}

	if v, ok := cfg.defined[EgressSubnets].(string); ok && v != "" {
		cidrs := strings.Split(v, ",")
		for _, cidr := range cidrs {
			if _, _, err := net.ParseCIDR(strings.TrimSpace(cidr)); err != nil {
				return errors.Annotatef(err, "invalid egress subnet: %v", cidr)
			}
			if cidr == "0.0.0.0/0" {
				return errors.Errorf("CIDR %q not allowed", cidr)
			}
		}
	}

	if v, ok := cfg.defined[FanConfig].(string); ok && v != "" {
		_, err := network.ParseFanConfig(v)
		if err != nil {
			return err
		}
	}

	if v, ok := cfg.defined[ContainerNetworkingMethod].(string); ok {
		switch v {
		case "fan":
			if cfg, err := cfg.FanConfig(); err != nil || cfg == nil {
				return errors.New("container-networking-method cannot be set to 'fan' without fan-config set")
			}
		case "provider": // TODO(wpk) FIXME we should check that the provider supports this setting!
		case "local":
		case "": // We'll try to autoconfigure it
		default:
			return fmt.Errorf("Invalid value for container-networking-method - %v", v)
		}
	}

	if raw, ok := cfg.defined[CloudInitUserDataKey].(string); ok && raw != "" {
		userDataMap, err := ensureStringMaps(raw)
		if err != nil {
			return errors.Annotate(err, "cloudinit-userdata")
		}

		// if there packages, ensure they are strings
		if packages, ok := userDataMap["packages"].([]interface{}); ok {
			for _, v := range packages {
				checker := schema.String()
				if _, err := checker.Coerce(v, nil); err != nil {
					return errors.Annotate(err, "cloudinit-userdata: packages must be a list of strings")
				}
			}
		}

		// error if users is specified
		if _, ok := userDataMap["users"]; ok {
			return errors.New("cloudinit-userdata: users not allowed")
		}

		// error if runcmd is specified
		if _, ok := userDataMap["runcmd"]; ok {
			return errors.New("cloudinit-userdata: runcmd not allowed, use preruncmd or postruncmd instead")
		}

		// error if bootcmd is specified
		if _, ok := userDataMap["bootcmd"]; ok {
			return errors.New("cloudinit-userdata: bootcmd not allowed")
		}
	}

	if raw, ok := cfg.defined[ContainerInheritPropertiesKey].(string); ok && raw != "" {
		rawProperties := strings.Split(raw, ",")
		propertySet := set.NewStrings()
		for _, prop := range rawProperties {
			propertySet.Add(strings.TrimSpace(prop))
		}
		whiteListSet := set.NewStrings("apt-primary", "apt-sources", "apt-security", "ca-certs")
		diffSet := propertySet.Difference(whiteListSet)

		if !diffSet.IsEmpty() {
			return errors.Errorf("container-inherit-properties: %s not allowed", strings.Join(diffSet.SortedValues(), ", "))
		}
	}

	if err := cfg.validateCharmHubURL(); err != nil {
		return errors.Trace(err)
	}

	if err := cfg.validateDefaultSpace(); err != nil {
		return errors.Trace(err)
	}

	if err := cfg.validateDefaultBase(); err != nil {
		return errors.Trace(err)
	}

	if err := cfg.validateMode(); err != nil {
		return errors.Trace(err)
	}

	if err := cfg.validateCIDRs(cfg.SSHAllow(), true); err != nil {
		return errors.Trace(err)
	}

	if err := cfg.validateCIDRs(cfg.SAASIngressAllow(), false); err != nil {
		return errors.Trace(err)
	}

	if err := cfg.validateLoggingOutput(); err != nil {
		return errors.Trace(err)
	}

	if err := cfg.validateNumProvisionWorkers(); err != nil {
		return errors.Trace(err)
	}

	if err := cfg.validateNumContainerProvisionWorkers(); err != nil {
		return errors.Trace(err)
	}

	if old != nil {
		// Check the immutable config values.  These can't change
		for _, attr := range immutableAttributes {
			oldv, ok := old.defined[attr]
			if !ok {
				continue
			}
			if newv := cfg.defined[attr]; newv != oldv {
				return fmt.Errorf("cannot change %s from %#v to %#v", attr, oldv, newv)
			}
		}
		if _, oldFound := old.AgentVersion(); oldFound {
			if _, newFound := cfg.AgentVersion(); !newFound {
				return errors.New("cannot clear agent-version")
			}
		}
		if _, oldFound := old.CharmHubURL(); oldFound {
			if _, newFound := cfg.CharmHubURL(); !newFound {
				return errors.New("cannot clear charmhub-url")
			}
		}
		// apt-mirror can't be set back to "" if it has previously been set.
		if old.AptMirror() != "" && cfg.AptMirror() == "" {
			return errors.New("cannot clear apt-mirror")
		}
	}

	// The user shouldn't specify both old and new proxy values.
	if cfg.HasLegacyProxy() && cfg.HasJujuProxy() {
		return errors.New("cannot specify both legacy proxy values and juju proxy values")
	}

	cfg.defined = ProcessDeprecatedAttributes(cfg.defined)
	return nil
}

// ensureStringMaps takes in a string and returns YAML in a map
// where all keys of any nested maps are strings.
func ensureStringMaps(in string) (map[string]interface{}, error) {
	userDataMap := make(map[string]interface{})
	if err := yaml.Unmarshal([]byte(in), &userDataMap); err != nil {
		return nil, errors.Annotate(err, "must be valid YAML")
	}
	out, err := utils.ConformYAML(userDataMap)
	if err != nil {
		return nil, err
	}
	return out.(map[string]interface{}), nil
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
	case []string:
		return len(val) == 0
	case map[string]string:
		return len(val) == 0
	}
	panic(fmt.Errorf("unexpected type %T in configuration", val))
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

// Type returns the model's cloud provider type.
func (c *Config) Type() string {
	return c.mustString(TypeKey)
}

// Name returns the model name.
func (c *Config) Name() string {
	return c.mustString(NameKey)
}

// UUID returns the uuid for the model.
func (c *Config) UUID() string {
	return c.mustString(UUIDKey)
}

func (c *Config) validateDefaultSpace() error {
	if raw, ok := c.defined[DefaultSpace]; ok {
		if v, ok := raw.(string); ok {
			if v == "" {
				return nil
			}
			if !names.IsValidSpace(v) {
				return errors.NotValidf("default space name %q", raw)
			}
		} else {
			return errors.NotValidf("type for default space name %v", raw)
		}
	}
	return nil
}

// DefaultSpace returns the name of the space for to be used
// for endpoint bindings that are not explicitly set.
func (c *Config) DefaultSpace() string {
	return c.asString(DefaultSpace)
}

func (c *Config) validateDefaultBase() error {
	defaultBase, configured := c.DefaultBase()
	if !configured {
		return nil
	}

	parsedBase, err := corebase.ParseBaseFromString(defaultBase)
	if err != nil {
		return errors.Annotatef(err, "invalid default base %q", defaultBase)
	}

	supported, err := corebase.WorkloadBases(time.Now(), corebase.Base{}, "")
	if err != nil {
		return errors.Annotate(err, "cannot read supported bases")
	}
	logger.Tracef("supported bases %s", supported)
	var found bool
	for _, supportedBase := range supported {
		if parsedBase.IsCompatible(supportedBase) {
			found = true
			break
		}
	}
	if !found {
		return errors.NotSupportedf("base %q", parsedBase.DisplayString())
	}
	return nil
}

// DefaultBase returns the configured default base for the model, and whether
// the default base was explicitly configured on the environment.
func (c *Config) DefaultBase() (string, bool) {
	s, ok := c.defined[DefaultBaseKey]
	if !ok {
		return "", false
	}
	switch s := s.(type) {
	case string:
		return s, s != ""
	default:
		logger.Errorf("invalid default-base: %q", s)
		return "", false
	}
}

// SecretBackend returns the secret backend name.
func (c *Config) SecretBackend() string {
	value, _ := c.defined[SecretBackendKey].(string)
	return value
}

// AuthorizedKeys returns the content for ssh's authorized_keys file.
func (c *Config) AuthorizedKeys() string {
	value, _ := c.defined[AuthorizedKeysKey].(string)
	return value
}

// ProxySSH returns a flag indicating whether SSH commands
// should be proxied through the API server.
func (c *Config) ProxySSH() bool {
	value, _ := c.defined["proxy-ssh"].(bool)
	return value
}

// NetBondReconfigureDelay returns the duration in seconds that should be
// passed to the bridge script when bridging bonded interfaces.
func (c *Config) NetBondReconfigureDelay() int {
	value, _ := c.defined[NetBondReconfigureDelayKey].(int)
	return value
}

// ContainerNetworkingMethod returns the method with which
// containers network should be set up.
func (c *Config) ContainerNetworkingMethod() string {
	return c.asString(ContainerNetworkingMethod)
}

// LegacyProxySettings returns all four proxy settings; http, https, ftp, and no
// proxy. These are considered legacy as using these values will cause the environment
// to be updated, which has shown to not work in many cases. It is being kept to avoid
// breaking environments where it is sufficient.
func (c *Config) LegacyProxySettings() proxy.Settings {
	return proxy.Settings{
		Http:    c.HTTPProxy(),
		Https:   c.HTTPSProxy(),
		Ftp:     c.FTPProxy(),
		NoProxy: c.NoProxy(),
	}
}

// HasLegacyProxy returns true if there is any proxy set using the old legacy proxy keys.
func (c *Config) HasLegacyProxy() bool {
	// We exclude the no proxy value as it has default value.
	return c.HTTPProxy() != "" ||
		c.HTTPSProxy() != "" ||
		c.FTPProxy() != ""
}

// HasJujuProxy returns true if there is any proxy set using the new juju-proxy keys.
func (c *Config) HasJujuProxy() bool {
	// We exclude the no proxy value as it has default value.
	return c.JujuHTTPProxy() != "" ||
		c.JujuHTTPSProxy() != "" ||
		c.JujuFTPProxy() != ""
}

// JujuProxySettings returns all four proxy settings that have been set using the
// juju- prefixed proxy settings. These values determine the current best practice
// for proxies.
func (c *Config) JujuProxySettings() proxy.Settings {
	return proxy.Settings{
		Http:    c.JujuHTTPProxy(),
		Https:   c.JujuHTTPSProxy(),
		Ftp:     c.JujuFTPProxy(),
		NoProxy: c.JujuNoProxy(),
	}
}

// HTTPProxy returns the legacy http proxy for the model.
func (c *Config) HTTPProxy() string {
	return c.asString(HTTPProxyKey)
}

// HTTPSProxy returns the legacy https proxy for the model.
func (c *Config) HTTPSProxy() string {
	return c.asString(HTTPSProxyKey)
}

// FTPProxy returns the legacy ftp proxy for the model.
func (c *Config) FTPProxy() string {
	return c.asString(FTPProxyKey)
}

// NoProxy returns the legacy  'no-proxy' for the model.
func (c *Config) NoProxy() string {
	return c.asString(NoProxyKey)
}

// JujuHTTPProxy returns the http proxy for the model.
func (c *Config) JujuHTTPProxy() string {
	return c.asString(JujuHTTPProxyKey)
}

// JujuHTTPSProxy returns the https proxy for the model.
func (c *Config) JujuHTTPSProxy() string {
	return c.asString(JujuHTTPSProxyKey)
}

// JujuFTPProxy returns the ftp proxy for the model.
func (c *Config) JujuFTPProxy() string {
	return c.asString(JujuFTPProxyKey)
}

// JujuNoProxy returns the 'no-proxy' for the model.
// This value can contain CIDR values.
func (c *Config) JujuNoProxy() string {
	return c.asString(JujuNoProxyKey)
}

func (c *Config) getWithFallback(key, fallback1, fallback2 string) string {
	value := c.asString(key)
	if value == "" {
		value = c.asString(fallback1)
	}
	if value == "" {
		value = c.asString(fallback2)
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
		Http:    c.AptHTTPProxy(),
		Https:   c.AptHTTPSProxy(),
		Ftp:     c.AptFTPProxy(),
		NoProxy: c.AptNoProxy(),
	}
}

// AptHTTPProxy returns the apt http proxy for the model.
// Falls back to the default http-proxy if not specified.
func (c *Config) AptHTTPProxy() string {
	return addSchemeIfMissing("http", c.getWithFallback(AptHTTPProxyKey, JujuHTTPProxyKey, HTTPProxyKey))
}

// AptHTTPSProxy returns the apt https proxy for the model.
// Falls back to the default https-proxy if not specified.
func (c *Config) AptHTTPSProxy() string {
	return addSchemeIfMissing("https", c.getWithFallback(AptHTTPSProxyKey, JujuHTTPSProxyKey, HTTPSProxyKey))
}

// AptFTPProxy returns the apt ftp proxy for the model.
// Falls back to the default ftp-proxy if not specified.
func (c *Config) AptFTPProxy() string {
	return addSchemeIfMissing("ftp", c.getWithFallback(AptFTPProxyKey, JujuFTPProxyKey, FTPProxyKey))
}

// AptNoProxy returns the 'apt-no-proxy' for the model.
func (c *Config) AptNoProxy() string {
	value := c.asString(AptNoProxyKey)
	if value == "" {
		if c.HasLegacyProxy() {
			value = c.asString(NoProxyKey)
		} else {
			value = c.asString(JujuNoProxyKey)
		}
	}
	return value
}

// AptMirror sets the apt mirror for the model.
func (c *Config) AptMirror() string {
	return c.asString(AptMirrorKey)
}

// SnapProxySettings returns the two proxy settings; http, and https.
func (c *Config) SnapProxySettings() proxy.Settings {
	return proxy.Settings{
		Http:  c.SnapHTTPProxy(),
		Https: c.SnapHTTPSProxy(),
	}
}

// SnapHTTPProxy returns the snap http proxy for the model.
func (c *Config) SnapHTTPProxy() string {
	return c.asString(SnapHTTPProxyKey)
}

// SnapHTTPSProxy returns the snap https proxy for the model.
func (c *Config) SnapHTTPSProxy() string {
	return c.asString(SnapHTTPSProxyKey)
}

// SnapStoreProxy returns the snap store proxy for the model.
func (c *Config) SnapStoreProxy() string {
	return c.asString(SnapStoreProxyKey)
}

// SnapStoreAssertions returns the snap store assertions for the model.
func (c *Config) SnapStoreAssertions() string {
	return c.asString(SnapStoreAssertionsKey)
}

// SnapStoreProxyURL returns the snap store proxy URL for the model.
func (c *Config) SnapStoreProxyURL() string {
	return c.asString(SnapStoreProxyURLKey)
}

// LogFwdSyslog returns the syslog forwarding config.
func (c *Config) LogFwdSyslog() (*syslog.RawConfig, bool) {
	partial := false
	var lfCfg syslog.RawConfig

	if s, ok := c.defined[LogForwardEnabled]; ok {
		partial = true
		lfCfg.Enabled = s.(bool)
	}

	if s, ok := c.defined[LogFwdSyslogHost]; ok && s != "" {
		partial = true
		lfCfg.Host = s.(string)
	}

	if s, ok := c.defined[LogFwdSyslogCACert]; ok && s != "" {
		partial = true
		lfCfg.CACert = s.(string)
	}

	if s, ok := c.defined[LogFwdSyslogClientCert]; ok && s != "" {
		partial = true
		lfCfg.ClientCert = s.(string)
	}

	if s, ok := c.defined[LogFwdSyslogClientKey]; ok && s != "" {
		partial = true
		lfCfg.ClientKey = s.(string)
	}

	if !partial {
		return nil, false
	}
	return &lfCfg, true
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
	if v, ok := c.defined[AgentVersionKey].(string); ok {
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

// ImageMetadataURL returns the URL at which the metadata used to locate image
// ids is located, and whether it has been set.
func (c *Config) ImageMetadataURL() (string, bool) {
	if url, ok := c.defined[ImageMetadataURLKey]; ok && url != "" {
		return url.(string), true
	}
	return "", false
}

// ImageMetadataDefaultsDisabled returns whether or not default image metadata
// sources are disabled. Useful for airgapped installations.
func (c *Config) ImageMetadataDefaultsDisabled() bool {
	val, ok := c.defined[ImageMetadataDefaultsDisabledKey].(bool)
	if !ok {
		// defaults to false.
		return false
	}
	return val
}

// ContainerImageMetadataURL returns the URL at which the metadata used to
// locate container OS image ids is located, and whether it has been set.
func (c *Config) ContainerImageMetadataURL() (string, bool) {
	if url, ok := c.defined[ContainerImageMetadataURLKey]; ok && url != "" {
		return url.(string), true
	}
	return "", false
}

// ContainerImageMetadataDefaultsDisabled returns whether or not default image metadata
// sources are disabled for containers. Useful for airgapped installations.
func (c *Config) ContainerImageMetadataDefaultsDisabled() bool {
	val, ok := c.defined[ContainerImageMetadataDefaultsDisabledKey].(bool)
	if !ok {
		// defaults to false.
		return false
	}
	return val
}

// Development returns whether the environment is in development mode.
func (c *Config) Development() bool {
	value, _ := c.defined["development"].(bool)
	return value
}

// EnableOSRefreshUpdate returns whether or not newly provisioned
// instances should run their respective OS's update capability.
func (c *Config) EnableOSRefreshUpdate() bool {
	val, ok := c.defined["enable-os-refresh-update"].(bool)
	if !ok {
		return true
	}
	return val
}

// EnableOSUpgrade returns whether or not newly provisioned instances
// should run their respective OS's upgrade capability.
func (c *Config) EnableOSUpgrade() bool {
	val, ok := c.defined["enable-os-upgrade"].(bool)
	if !ok {
		return true
	}
	return val
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

// BackupDir returns the configuration string for the temporary files
// backup.
func (c *Config) BackupDir() string {
	return c.asString(BackupDirKey)
}

// AutomaticallyRetryHooks returns whether we should automatically retry hooks.
// By default this should be true.
func (c *Config) AutomaticallyRetryHooks() bool {
	val, ok := c.defined["automatically-retry-hooks"].(bool)
	if !ok {
		return true
	}
	return val
}

// TransmitVendorMetrics returns whether the controller sends charm-collected metrics
// in this model for anonymized aggregate analytics. By default this should be true.
func (c *Config) TransmitVendorMetrics() bool {
	val, ok := c.defined[TransmitVendorMetricsKey].(bool)
	if !ok {
		return true
	}
	return val
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

// NumProvisionWorkers returns the number of provisioner workers to use.
func (c *Config) NumProvisionWorkers() int {
	value, _ := c.defined[NumProvisionWorkersKey].(int)
	return value
}

const (
	MaxNumProvisionWorkers          = 100
	MaxNumContainerProvisionWorkers = 25
)

// validateNumProvisionWorkers ensures the number cannot be set to
// more than 100.
// TODO: (hml) 26-Feb-2024
// Once we can better link the controller config and the model config,
// allow the max value to be set in the controller config.
func (c *Config) validateNumProvisionWorkers() error {
	value, ok := c.defined[NumProvisionWorkersKey].(int)
	if ok && value > MaxNumProvisionWorkers {
		return errors.Errorf("%s: must be less than %d", NumProvisionWorkersKey, MaxNumProvisionWorkers)
	}
	return nil
}

// NumContainerProvisionWorkers returns the number of container provisioner
// workers to use.
func (c *Config) NumContainerProvisionWorkers() int {
	value, _ := c.defined[NumContainerProvisionWorkersKey].(int)
	return value
}

// validateNumContainerProvisionWorkers ensures the number cannot be set to
// more than 25.
// TODO: (hml) 26-Feb-2024
// Once we can better link the controller config and the model config,
// allow the max value to be set in the controller config.
func (c *Config) validateNumContainerProvisionWorkers() error {
	value, ok := c.defined[NumContainerProvisionWorkersKey].(int)
	if ok && value > MaxNumContainerProvisionWorkers {
		return errors.Errorf("%s: must be less than %d", NumContainerProvisionWorkersKey, MaxNumContainerProvisionWorkers)
	}
	return nil
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
// bootstrapping or upgrading an environment.
func (c *Config) AgentStream() string {
	v, _ := c.defined[AgentStreamKey].(string)
	if v != "" {
		return v
	}
	return "released"
}

// ContainerImageStream returns the simplestreams stream used to identify which
// image ids to search when starting a container.
func (c *Config) ContainerImageStream() string {
	v, _ := c.defined[ContainerImageStreamKey].(string)
	if v != "" {
		return v
	}
	return "released"
}

// CharmHubURL returns the URL to use for CharmHub API calls.
func (c *Config) CharmHubURL() (string, bool) {
	if v, ok := c.defined[CharmHubURLKey].(string); ok && v != "" {
		return v, true
	}
	return charmhub.DefaultServerURL, false
}

func (c *Config) validateCharmHubURL() error {
	if v, ok := c.defined[CharmHubURLKey].(string); ok {
		if v == "" {
			return errors.NotValidf("charm-hub url")
		}
		if _, err := url.ParseRequestURI(v); err != nil {
			return errors.NotValidf("charm-hub url %q", v)
		}
	}
	return nil
}

const (
	// RequiresPromptsMode is used to tell clients interacting with
	// model that confirmation prompts are required when removing
	// potentially important resources
	RequiresPromptsMode = "requires-prompts"

	// StrictMode is currently unused
	// TODO(jack-w-shaw) remove this mode
	StrictMode = "strict"
)

var allModes = set.NewStrings(RequiresPromptsMode, StrictMode)

// Mode returns a set of mode types for the configuration.
// Only one option exists at the moment ('requires-prompts')
func (c *Config) Mode() (set.Strings, bool) {
	modes, ok := c.defined[ModeKey]
	if !ok {
		return set.NewStrings(), false
	}
	if m, ok := modes.(string); ok {
		s := set.NewStrings()
		for _, v := range strings.Split(strings.TrimSpace(m), ",") {
			if v == "" {
				continue
			}
			s.Add(strings.TrimSpace(v))
		}
		if s.Size() > 0 {
			return s, true
		}
	}

	return set.NewStrings(), false
}

func (c *Config) validateMode() error {
	modes, _ := c.Mode()
	difference := modes.Difference(allModes)
	if !difference.IsEmpty() {
		return errors.NotValidf("mode(s) %q", strings.Join(difference.SortedValues(), ", "))
	}
	return nil
}

// SSHAllow returns a slice of CIDRs from which machines in
// this model will accept connections to the SSH service
func (c *Config) SSHAllow() []string {
	allowList, ok := c.defined[SSHAllowKey].(string)
	if !ok {
		return []string{"0.0.0.0/0", "::/0"}
	}
	if allowList == "" {
		return []string{}
	}
	return strings.Split(allowList, ",")
}

// SAASIngressAllow returns a slice of CIDRs specifying what
// ingress can be applied to offers in this model
func (c *Config) SAASIngressAllow() []string {
	allowList, ok := c.defined[SAASIngressAllowKey].(string)
	if !ok {
		return []string{"0.0.0.0/0"}
	}
	if allowList == "" {
		return []string{}
	}
	return strings.Split(allowList, ",")
}

func (c *Config) validateCIDRs(cidrs []string, allowEmpty bool) error {
	if len(cidrs) == 0 && !allowEmpty {
		return errors.NotValidf("empty cidrs")
	}
	for _, cidr := range cidrs {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return errors.NotValidf("cidr %q", cidr)
		}
	}
	return nil
}

// LoggingOutput is a for determining the destination of output for
// logging.
func (c *Config) LoggingOutput() ([]string, bool) {
	outputs, ok := c.defined[LoggingOutputKey]
	if !ok {
		return []string{}, false
	}
	if m, ok := outputs.(string); ok {
		s := set.NewStrings()
		for _, v := range strings.Split(strings.TrimSpace(m), ",") {
			if v == "" {
				continue
			}
			s.Add(strings.TrimSpace(v))
		}
		if s.Size() > 0 {
			return s.SortedValues(), true
		}
	}
	return []string{}, false
}

func (c *Config) validateLoggingOutput() error {
	outputs, _ := c.LoggingOutput()
	for _, output := range outputs {
		switch strings.TrimSpace(output) {
		case corelogger.DatabaseName, corelogger.SyslogName:
		default:
			return errors.NotValidf("logging-output %q", output)
		}
	}
	return nil
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
// source for the model.
func (c *Config) StorageDefaultBlockSource() (string, bool) {
	bs := c.asString(StorageDefaultBlockSourceKey)
	return bs, bs != ""
}

// StorageDefaultFilesystemSource returns the default filesystem
// storage source for the model.
func (c *Config) StorageDefaultFilesystemSource() (string, bool) {
	bs := c.asString(StorageDefaultFilesystemSourceKey)
	return bs, bs != ""
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

// MaxStatusHistoryAge is the maximum age of status history entries
// before being pruned.
func (c *Config) MaxStatusHistoryAge() time.Duration {
	// Value has already been validated.
	val, _ := time.ParseDuration(c.mustString(MaxStatusHistoryAge))
	return val
}

// MaxStatusHistorySizeMB is the maximum size in MiB which the status history
// collection can grow to before being pruned.
func (c *Config) MaxStatusHistorySizeMB() uint {
	// Value has already been validated.
	val, _ := utils.ParseSize(c.mustString(MaxStatusHistorySize))
	return uint(val)
}

func (c *Config) MaxActionResultsAge() time.Duration {
	// Value has already been validated.
	val, _ := time.ParseDuration(c.mustString(MaxActionResultsAge))
	return val
}

func (c *Config) MaxActionResultsSizeMB() uint {
	// Value has already been validated.
	val, _ := utils.ParseSize(c.mustString(MaxActionResultsSize))
	return uint(val)
}

// UpdateStatusHookInterval is how often to run the charm
// update-status hook.
func (c *Config) UpdateStatusHookInterval() time.Duration {
	// Value has already been validated.
	val, _ := time.ParseDuration(c.asString(UpdateStatusHookInterval))
	return val
}

// EgressSubnets are the source addresses from which traffic from this model
// originates if the model is deployed such that NAT or similar is in use.
func (c *Config) EgressSubnets() []string {
	raw := c.asString(EgressSubnets)
	if raw == "" {
		return []string{}
	}
	// Value has already been validated.
	rawAddr := strings.Split(raw, ",")
	result := make([]string, len(rawAddr))
	for i, addr := range rawAddr {
		result[i] = strings.TrimSpace(addr)
	}
	return result
}

// FanConfig is the configuration of FAN network running in the model.
func (c *Config) FanConfig() (network.FanConfig, error) {
	// At this point we are sure that the line is valid.
	return network.ParseFanConfig(c.asString(FanConfig))
}

// CloudInitUserData returns a copy of the raw user data attributes
// that were specified by the user.
func (c *Config) CloudInitUserData() map[string]interface{} {
	raw := c.asString(CloudInitUserDataKey)
	if raw == "" {
		return nil
	}
	// The raw data has already passed Validate()
	conformingUserDataMap, _ := ensureStringMaps(raw)
	return conformingUserDataMap
}

// ContainerInheritProperties returns a copy of the raw user data keys
// that were specified by the user.
func (c *Config) ContainerInheritProperties() string {
	return c.asString(ContainerInheritPropertiesKey)
}

// LXDSnapChannel returns the channel to be used when installing LXD from a snap.
func (c *Config) LXDSnapChannel() string {
	return c.asString(LXDSnapChannel)
}

// Telemetry returns whether telemetry is enabled for the model.
func (c *Config) Telemetry() bool {
	value, _ := c.defined[DisableTelemetryKey].(bool)
	return !value
}

// ContainerImageReference returns the configured full image reference string for the container
// and a boolean indicating whether the value was explicitly set in the config.
func (c *Config) ContainerImageReference() (string, bool) {
	if imageRepo, ok := c.defined[ContainerImageReference]; ok && imageRepo != "" {
		return imageRepo.(string), true
	}
	return "", false
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
	combinedSchema, err := Schema(nil)
	if err != nil {
		panic(err)
	}
	fs, _, err := combinedSchema.ValidationSchema()
	if err != nil {
		panic(err)
	}
	return fs
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
	AgentVersionKey:   schema.Omit,
	AuthorizedKeysKey: schema.Omit,
	ExtraInfoKey:      schema.Omit,

	LogForwardEnabled:      schema.Omit,
	LogFwdSyslogHost:       schema.Omit,
	LogFwdSyslogCACert:     schema.Omit,
	LogFwdSyslogClientCert: schema.Omit,
	LogFwdSyslogClientKey:  schema.Omit,
	LoggingOutputKey:       schema.Omit,

	// Storage related config.
	// Environ providers will specify their own defaults.
	StorageDefaultBlockSourceKey:      schema.Omit,
	StorageDefaultFilesystemSourceKey: schema.Omit,

	"firewall-mode":     schema.Omit,
	SSHAllowKey:         schema.Omit,
	SAASIngressAllowKey: schema.Omit,

	"logging-config":                schema.Omit,
	ProvisionerHarvestModeKey:       schema.Omit,
	NumProvisionWorkersKey:          schema.Omit,
	NumContainerProvisionWorkersKey: schema.Omit,
	HTTPProxyKey:                    schema.Omit,
	HTTPSProxyKey:                   schema.Omit,
	FTPProxyKey:                     schema.Omit,
	NoProxyKey:                      schema.Omit,
	JujuHTTPProxyKey:                schema.Omit,
	JujuHTTPSProxyKey:               schema.Omit,
	JujuFTPProxyKey:                 schema.Omit,
	JujuNoProxyKey:                  schema.Omit,
	AptHTTPProxyKey:                 schema.Omit,
	AptHTTPSProxyKey:                schema.Omit,
	AptFTPProxyKey:                  schema.Omit,
	AptNoProxyKey:                   schema.Omit,
	SnapHTTPProxyKey:                schema.Omit,
	SnapHTTPSProxyKey:               schema.Omit,
	SnapStoreProxyKey:               schema.Omit,
	SnapStoreAssertionsKey:          schema.Omit,
	SnapStoreProxyURLKey:            schema.Omit,
	AptMirrorKey:                    schema.Omit,
	AgentStreamKey:                  schema.Omit,
	ResourceTagsKey:                 schema.Omit,
	"cloudimg-base-url":             schema.Omit,
	"enable-os-refresh-update":      schema.Omit,
	"enable-os-upgrade":             schema.Omit,
	DefaultBaseKey:                  schema.Omit,
	"development":                   schema.Omit,
	"ssl-hostname-verification":     schema.Omit,
	"proxy-ssh":                     schema.Omit,
	"disable-network-management":    schema.Omit,
	IgnoreMachineAddresses:          schema.Omit,
	AutomaticallyRetryHooks:         schema.Omit,
	TestModeKey:                     schema.Omit,
	DisableTelemetryKey:             schema.Omit,
	ModeKey:                         schema.Omit,
	TransmitVendorMetricsKey:        schema.Omit,
	NetBondReconfigureDelayKey:      schema.Omit,
	ContainerNetworkingMethod:       schema.Omit,
	MaxStatusHistoryAge:             schema.Omit,
	MaxStatusHistorySize:            schema.Omit,
	MaxActionResultsAge:             schema.Omit,
	MaxActionResultsSize:            schema.Omit,
	UpdateStatusHookInterval:        schema.Omit,
	EgressSubnets:                   schema.Omit,
	FanConfig:                       schema.Omit,
	CloudInitUserDataKey:            schema.Omit,
	ContainerInheritPropertiesKey:   schema.Omit,
	BackupDirKey:                    schema.Omit,
	DefaultSpace:                    schema.Omit,
	LXDSnapChannel:                  schema.Omit,
	CharmHubURLKey:                  schema.Omit,

	AgentMetadataURLKey:                       schema.Omit,
	ImageStreamKey:                            schema.Omit,
	ImageMetadataURLKey:                       schema.Omit,
	ImageMetadataDefaultsDisabledKey:          schema.Omit,
	ContainerImageStreamKey:                   schema.Omit,
	ContainerImageMetadataURLKey:              schema.Omit,
	ContainerImageMetadataDefaultsDisabledKey: schema.Omit,
	ContainerImageReference:                   schema.Omit,
}

func allowEmpty(attr string) bool {
	return alwaysOptional[attr] == "" || alwaysOptional[attr] == schema.Omit
}

// allDefaults returns a schema.Defaults that contains
// defaults to be used when creating a new config with
// UseDefaults.
func allDefaults() schema.Defaults {
	d := schema.Defaults{}
	configDefaults := ConfigDefaults()
	for attr, val := range configDefaults {
		d[attr] = val
	}
	for attr, val := range alwaysOptional {
		if developerConfigValue(attr) {
			continue
		}
		if _, ok := d[attr]; !ok {
			d[attr] = val
		}
	}
	return d
}

// immutableAttributes holds those attributes
// which are not allowed to change in the lifetime
// of an environment.
var immutableAttributes = []string{
	NameKey,
	TypeKey,
	UUIDKey,
	"firewall-mode",
	CharmHubURLKey,
}

var (
	initSchema          sync.Once
	allFields           schema.Fields
	defaultsWhenParsing schema.Defaults
	withDefaultsChecker schema.Checker
	noDefaultsChecker   schema.Checker
)

func ensureSchema() {
	initSchema.Do(func() {
		allFields = fields()
		defaultsWhenParsing = allDefaults()
		withDefaultsChecker = schema.FieldMap(allFields, defaultsWhenParsing)
		noDefaultsChecker = schema.FieldMap(allFields, alwaysOptional)
	})
}

// ValidateUnknownAttrs checks the unknown attributes of the config against
// the supplied fields and defaults, and returns an error if any fails to
// validate. Unknown fields are warned about, but preserved, on the basis
// that they are reasonably likely to have been written by or for a version
// of juju that does recognise the fields, but that their presence is still
// anomalous to some degree and should be flagged (and that there is thereby
// a mechanism for observing fields that really are typos etc).
func (c *Config) ValidateUnknownAttrs(extrafields schema.Fields, defaults schema.Defaults) (map[string]interface{}, error) {
	attrs := c.UnknownAttrs()
	checker := schema.FieldMap(extrafields, defaults)
	coerced, err := checker.Coerce(attrs, nil)
	if err != nil {
		logger.Debugf("coercion failed attributes: %#v, checker: %#v, %v", attrs, checker, err)
		return nil, err
	}
	result := coerced.(map[string]interface{})
	for name, value := range attrs {
		if extrafields[name] == nil {
			// We know this name isn't in the global fields, or it wouldn't be
			// an UnknownAttr, it also appears to not be in the extra fields
			// that are provider specific.  Check to see if an alternative
			// spelling is in either the extra fields or the core fields.
			if val, isString := value.(string); isString && val != "" {
				// only warn about attributes with non-empty string values
				altName := strings.Replace(name, "_", "-", -1)
				if extrafields[altName] != nil || allFields[altName] != nil {
					logger.Warningf("unknown config field %q, did you mean %q?", name, altName)
				} else {
					logger.Warningf("unknown config field %q", name)
				}
			}
			result[name] = value
			// The only allowed types for unknown attributes are string, int,
			// float, bool and []interface{} (which is really []string)
			switch t := value.(type) {
			case string:
				continue
			case int:
				continue
			case bool:
				continue
			case float32:
				continue
			case float64:
				continue
			case []interface{}:
				for _, val := range t {
					if _, ok := val.(string); !ok {
						return nil, errors.Errorf("%s: unknown type (%v)", name, value)
					}
				}
				continue
			default:
				return nil, errors.Errorf("%s: unknown type (%q)", name, value)
			}
		}
	}
	return result, nil
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
	addIfNotEmpty(settings, HTTPProxyKey, proxySettings.Http)
	addIfNotEmpty(settings, HTTPSProxyKey, proxySettings.Https)
	addIfNotEmpty(settings, FTPProxyKey, proxySettings.Ftp)
	addIfNotEmpty(settings, NoProxyKey, proxySettings.NoProxy)
	return settings
}

// AptProxyConfigMap returns a map suitable to be applied to a Config to update
// proxy settings.
func AptProxyConfigMap(proxySettings proxy.Settings) map[string]interface{} {
	settings := make(map[string]interface{})
	addIfNotEmpty(settings, AptHTTPProxyKey, proxySettings.Http)
	addIfNotEmpty(settings, AptHTTPSProxyKey, proxySettings.Https)
	addIfNotEmpty(settings, AptFTPProxyKey, proxySettings.Ftp)
	addIfNotEmpty(settings, AptNoProxyKey, proxySettings.NoProxy)
	return settings
}

func developerConfigValue(name string) bool {
	if !featureflag.Enabled(feature.DeveloperMode) {
		switch name {
		// Add developer-mode keys here.
		}
	}
	return false
}

// Schema returns a configuration schema that includes both
// the given extra fields and all the fields defined in this package.
// It returns an error if extra defines any fields defined in this
// package.
func Schema(extra environschema.Fields) (environschema.Fields, error) {
	fields := make(environschema.Fields)
	for name, field := range configSchema {
		if developerConfigValue(name) {
			continue
		}
		if controller.ControllerOnlyAttribute(name) {
			return nil, errors.Errorf("config field %q clashes with controller config", name)
		}
		fields[name] = field
	}
	for name, field := range extra {
		if controller.ControllerOnlyAttribute(name) {
			return nil, errors.Errorf("config field %q clashes with controller config", name)
		}
		if _, ok := fields[name]; ok {
			return nil, errors.Errorf("config field %q clashes with global config", name)
		}
		fields[name] = field
	}
	return fields, nil
}

// configSchema holds information on all the fields defined by
// the config package.
var configSchema = environschema.Fields{
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
	AgentVersionKey: {
		Description: "The desired Juju agent version to use",
		Type:        environschema.Tstring,
		Group:       environschema.JujuGroup,
		Immutable:   true,
	},
	AptFTPProxyKey: {
		// TODO document acceptable format
		Description: "The APT FTP proxy for the model",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	AptHTTPProxyKey: {
		// TODO document acceptable format
		Description: "The APT HTTP proxy for the model",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	AptHTTPSProxyKey: {
		// TODO document acceptable format
		Description: "The APT HTTPS proxy for the model",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	AptNoProxyKey: {
		Description: "List of domain addresses not to be proxied for APT (comma-separated)",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	AptMirrorKey: {
		// TODO document acceptable format
		Description: "The APT mirror for the model",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	AuthorizedKeysKey: {
		Description: "Any authorized SSH public keys for the model, as found in a ~/.ssh/authorized_keys file",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	DefaultBaseKey: {
		Description: "The default base image to use for deploying charms, will act like --base when deploying charms",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	// TODO (jack-w-shaw) integrate this into mode
	"development": {
		Description: "Whether the model is in development mode",
		Type:        environschema.Tbool,
		Group:       environschema.EnvironGroup,
	},
	"disable-network-management": {
		Description: "Whether the provider should control networks (on MAAS models, set to true for MAAS to control networks",
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
	ExtraInfoKey: {
		Description: "Arbitrary user specified string data that is stored against the model.",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	"firewall-mode": {
		Description: `The mode to use for network firewalling.

'instance' requests the use of an individual firewall per instance.

'global' uses a single firewall for all instances (access
for a network port is enabled to one instance if any instance requires
that port).

'none' requests that no firewalling should be performed
inside the model. It's useful for clouds without support for either
global or per instance security groups.`,
		Type:      environschema.Tstring,
		Values:    []interface{}{FwInstance, FwGlobal, FwNone},
		Immutable: true,
		Group:     environschema.EnvironGroup,
	},
	FTPProxyKey: {
		Description: "The FTP proxy value to configure on instances, in the FTP_PROXY environment variable",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	HTTPProxyKey: {
		Description: "The HTTP proxy value to configure on instances, in the HTTP_PROXY environment variable",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	HTTPSProxyKey: {
		Description: "The HTTPS proxy value to configure on instances, in the HTTPS_PROXY environment variable",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	NoProxyKey: {
		Description: "List of domain addresses not to be proxied (comma-separated)",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	JujuFTPProxyKey: {
		Description: "The FTP proxy value to pass to charms in the JUJU_CHARM_FTP_PROXY environment variable",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	JujuHTTPProxyKey: {
		Description: "The HTTP proxy value to pass to charms in the JUJU_CHARM_HTTP_PROXY environment variable",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	JujuHTTPSProxyKey: {
		Description: "The HTTPS proxy value to pass to charms in the JUJU_CHARM_HTTPS_PROXY environment variable",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	JujuNoProxyKey: {
		Description: "List of domain addresses not to be proxied (comma-separated), may contain CIDRs. Passed to charms in the JUJU_CHARM_NO_PROXY environment variable",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	SnapHTTPProxyKey: {
		Description: "The HTTP proxy value for installing snaps",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	SnapHTTPSProxyKey: {
		Description: "The HTTPS proxy value for installing snaps",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	SnapStoreProxyKey: {
		Description: "The snap store proxy for installing snaps",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	SnapStoreAssertionsKey: {
		Description: "The assertions for the defined snap store proxy",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	SnapStoreProxyURLKey: {
		Description: "The URL for the defined snap store proxy",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	ImageMetadataURLKey: {
		Description: "The URL at which the metadata used to locate OS image ids is located",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	ImageStreamKey: {
		Description: `The simplestreams stream used to identify which image ids to search when starting an instance.`,
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	ImageMetadataDefaultsDisabledKey: {
		Description: `Whether default simplestreams sources are used for image metadata.`,
		Type:        environschema.Tbool,
		Group:       environschema.EnvironGroup,
	},
	ContainerImageMetadataURLKey: {
		Description: "The URL at which the metadata used to locate container OS image ids is located",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	ContainerImageStreamKey: {
		Description: `The simplestreams stream used to identify which image ids to search when starting a container.`,
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	ContainerImageMetadataDefaultsDisabledKey: {
		Description: `Whether default simplestreams sources are used for image metadata with containers.`,
		Type:        environschema.Tbool,
		Group:       environschema.EnvironGroup,
	},
	"logging-config": {
		Description: `The configuration string to use when configuring Juju agent logging (see http://godoc.org/github.com/juju/loggo#ParseConfigurationString for details)`,
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	NameKey: {
		Description: "The name of the current model",
		Type:        environschema.Tstring,
		Mandatory:   true,
		Immutable:   true,
		Group:       environschema.EnvironGroup,
	},
	ProvisionerHarvestModeKey: {
		// default: destroyed, but also depends on current setting of ProvisionerSafeModeKey
		Description: "What to do with unknown machines (default destroyed)",
		Type:        environschema.Tstring,
		Values:      []interface{}{"all", "none", "unknown", "destroyed"},
		Group:       environschema.EnvironGroup,
	},
	NumProvisionWorkersKey: {
		Description: "The number of provisioning workers to use per model",
		Type:        environschema.Tint,
		Group:       environschema.EnvironGroup,
	},
	NumContainerProvisionWorkersKey: {
		Description: "The number of container provisioning workers to use per machine",
		Type:        environschema.Tint,
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
	LogForwardEnabled: {
		Description: `Whether syslog forwarding is enabled.`,
		Type:        environschema.Tbool,
		Group:       environschema.EnvironGroup,
	},
	LogFwdSyslogHost: {
		Description: `The hostname:port of the syslog server.`,
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	LogFwdSyslogCACert: {
		Description: `The certificate of the CA that signed the syslog server certificate, in PEM format.`,
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	LogFwdSyslogClientCert: {
		Description: `The syslog client certificate in PEM format.`,
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	LogFwdSyslogClientKey: {
		Description: `The syslog client key in PEM format.`,
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	"ssl-hostname-verification": {
		Description: "Whether SSL hostname verification is enabled (default true)",
		Type:        environschema.Tbool,
		Group:       environschema.EnvironGroup,
	},
	StorageDefaultBlockSourceKey: {
		Description: "The default block storage source for the model",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	StorageDefaultFilesystemSourceKey: {
		Description: "The default filesystem storage source for the model",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	TestModeKey: {
		Description: `Whether the model is intended for testing.
If true, accessing the charm store does not affect statistical
data of the store. (default false)`,
		Type:  environschema.Tbool,
		Group: environschema.EnvironGroup,
	},
	DisableTelemetryKey: {
		Description: `Disable telemetry reporting of model information`,
		Type:        environschema.Tbool,
		Group:       environschema.EnvironGroup,
	},
	ModeKey: {
		Description: `Mode is a comma-separated list which sets the
mode the model should run in. So far only one is implemented
- If 'requires-prompts' is present, clients will ask for confirmation before removing
potentially valuable resources.
(default "")`,
		Type:  environschema.Tstring,
		Group: environschema.EnvironGroup,
	},
	SSHAllowKey: {
		Description: `SSH allowlist is a comma-separated list of CIDRs from
which machines in this model will accept connections to the SSH service.
Currently only the aws & openstack providers support ssh-allow`,
		Type:  environschema.Tstring,
		Group: environschema.EnvironGroup,
	},
	SAASIngressAllowKey: {
		Description: `Application-offer ingress allowlist is a comma-separated list of
CIDRs specifying what ingress can be applied to offers in this model.`,
		Type:  environschema.Tstring,
		Group: environschema.EnvironGroup,
	},
	TypeKey: {
		Description: "Type of model, e.g. local, ec2",
		Type:        environschema.Tstring,
		Mandatory:   true,
		Immutable:   true,
		Group:       environschema.EnvironGroup,
	},
	UUIDKey: {
		Description: "The UUID of the model",
		Type:        environschema.Tstring,
		Group:       environschema.JujuGroup,
		Immutable:   true,
	},
	AutomaticallyRetryHooks: {
		Description: "Determines whether the uniter should automatically retry failed hooks",
		Type:        environschema.Tbool,
		Group:       environschema.EnvironGroup,
	},
	TransmitVendorMetricsKey: {
		Description: "Determines whether metrics declared by charms deployed into this model are sent for anonymized aggregate analytics",
		Type:        environschema.Tbool,
		Group:       environschema.EnvironGroup,
	},
	NetBondReconfigureDelayKey: {
		Description: "The amount of time in seconds to sleep between ifdown and ifup when bridging",
		Type:        environschema.Tint,
		Group:       environschema.EnvironGroup,
	},
	ContainerNetworkingMethod: {
		Description: "Method of container networking setup - one of fan, provider, local",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	MaxStatusHistoryAge: {
		Description: "The maximum age for status history entries before they are pruned, in human-readable time format",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	MaxStatusHistorySize: {
		Description: "The maximum size for the status history collection, in human-readable memory format",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	MaxActionResultsAge: {
		Description: "The maximum age for action entries before they are pruned, in human-readable time format",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	MaxActionResultsSize: {
		Description: "The maximum size for the action collection, in human-readable memory format",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	UpdateStatusHookInterval: {
		Description: "How often to run the charm update-status hook, in human-readable time format (default 5m, range 1-60m)",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	EgressSubnets: {
		Description: "Source address(es) for traffic originating from this model",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	FanConfig: {
		Description: "Configuration for fan networking for this model",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	CloudInitUserDataKey: {
		Description: "Cloud-init user-data (in yaml format) to be added to userdata for new machines created in this model",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	ContainerInheritPropertiesKey: {
		Description: "List of properties to be copied from the host machine to new containers created in this model (comma-separated)",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	BackupDirKey: {
		Description: "Directory used to store the backup working directory",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	DefaultSpace: {
		Description: "The default network space used for application endpoints in this model",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	LXDSnapChannel: {
		Description: "The channel to use when installing LXD from a snap (cosmic and later)",
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	CharmHubURLKey: {
		Description: `The url for CharmHub API calls`,
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	LoggingOutputKey: {
		Description: `The logging output destination: database and/or syslog. (default "")`,
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	SecretBackendKey: {
		Description: `The name of the secret store backend. (default "auto")`,
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
	ContainerImageReference: {
		Description: `The container image repository. (default "")`,
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
}
