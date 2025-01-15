// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config

import (
	"github.com/juju/errors"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/internal/configschema"
)

// Schema returns a configuration schema that includes both
// the given extra fields and all the fields defined in this package.
// It returns an error if extra defines any fields defined in this
// package.
func Schema(extra configschema.Fields) (configschema.Fields, error) {
	fields := make(configschema.Fields)
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
var configSchema = configschema.Fields{
	AgentMetadataURLKey: {
		Description: "URL of private stream",
		Type:        configschema.Tstring,
		Group:       configschema.EnvironGroup,
	},
	AgentStreamKey: {
		Description: `Version of Juju to use for deploy/upgrades.`,
		Type:        configschema.Tstring,
		Group:       configschema.EnvironGroup,
	},
	AgentVersionKey: {
		Description: "The desired Juju agent version to use",
		Type:        configschema.Tstring,
		Group:       configschema.JujuGroup,
		Immutable:   true,
	},
	AptFTPProxyKey: {
		// TODO document acceptable format
		Description: "The APT FTP proxy for the model",
		Type:        configschema.Tstring,
		Group:       configschema.EnvironGroup,
	},
	AptHTTPProxyKey: {
		// TODO document acceptable format
		Description: "The APT HTTP proxy for the model",
		Type:        configschema.Tstring,
		Group:       configschema.EnvironGroup,
	},
	AptHTTPSProxyKey: {
		// TODO document acceptable format
		Description: "The APT HTTPS proxy for the model",
		Type:        configschema.Tstring,
		Group:       configschema.EnvironGroup,
	},
	AptNoProxyKey: {
		Description: "List of domain addresses not to be proxied for APT (comma-separated)",
		Type:        configschema.Tstring,
		Group:       configschema.EnvironGroup,
	},
	AptMirrorKey: {
		// TODO document acceptable format
		Description: "The APT mirror for the model",
		Type:        configschema.Tstring,
		Group:       configschema.EnvironGroup,
	},
	DefaultBaseKey: {
		Description: "The default base image to use for deploying charms, will act like --base when deploying charms",
		Type:        configschema.Tstring,
		Group:       configschema.EnvironGroup,
	},
	// TODO (jack-w-shaw) integrate this into mode
	DevelopmentKey: {
		Description: "Whether the model is in development mode",
		Type:        configschema.Tbool,
		Group:       configschema.EnvironGroup,
	},
	"disable-network-management": {
		Description: "Whether the provider should control networks (on MAAS models, set to true for MAAS to control networks",
		Type:        configschema.Tbool,
		Group:       configschema.EnvironGroup,
	},
	IgnoreMachineAddresses: {
		Description: "Whether the machine worker should discover machine addresses on startup",
		Type:        configschema.Tbool,
		Group:       configschema.EnvironGroup,
	},
	EnableOSRefreshUpdateKey: {
		Description: `Whether newly provisioned instances should run their respective OS's update capability.`,
		Type:        configschema.Tbool,
		Group:       configschema.EnvironGroup,
	},
	EnableOSUpgradeKey: {
		Description: `Whether newly provisioned instances should run their respective OS's upgrade capability.`,
		Type:        configschema.Tbool,
		Group:       configschema.EnvironGroup,
	},
	ExtraInfoKey: {
		Description: "Arbitrary user specified string data that is stored against the model.",
		Type:        configschema.Tstring,
		Group:       configschema.EnvironGroup,
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
		Type:      configschema.Tstring,
		Values:    []interface{}{FwInstance, FwGlobal, FwNone},
		Immutable: true,
		Group:     configschema.EnvironGroup,
	},
	FTPProxyKey: {
		Description: "The FTP proxy value to configure on instances, in the FTP_PROXY environment variable",
		Type:        configschema.Tstring,
		Group:       configschema.EnvironGroup,
	},
	HTTPProxyKey: {
		Description: "The HTTP proxy value to configure on instances, in the HTTP_PROXY environment variable",
		Type:        configschema.Tstring,
		Group:       configschema.EnvironGroup,
	},
	HTTPSProxyKey: {
		Description: "The HTTPS proxy value to configure on instances, in the HTTPS_PROXY environment variable",
		Type:        configschema.Tstring,
		Group:       configschema.EnvironGroup,
	},
	NoProxyKey: {
		Description: "List of domain addresses not to be proxied (comma-separated)",
		Type:        configschema.Tstring,
		Group:       configschema.EnvironGroup,
	},
	JujuFTPProxyKey: {
		Description: "The FTP proxy value to pass to charms in the JUJU_CHARM_FTP_PROXY environment variable",
		Type:        configschema.Tstring,
		Group:       configschema.EnvironGroup,
	},
	JujuHTTPProxyKey: {
		Description: "The HTTP proxy value to pass to charms in the JUJU_CHARM_HTTP_PROXY environment variable",
		Type:        configschema.Tstring,
		Group:       configschema.EnvironGroup,
	},
	JujuHTTPSProxyKey: {
		Description: "The HTTPS proxy value to pass to charms in the JUJU_CHARM_HTTPS_PROXY environment variable",
		Type:        configschema.Tstring,
		Group:       configschema.EnvironGroup,
	},
	JujuNoProxyKey: {
		Description: "List of domain addresses not to be proxied (comma-separated), may contain CIDRs. Passed to charms in the JUJU_CHARM_NO_PROXY environment variable",
		Type:        configschema.Tstring,
		Group:       configschema.EnvironGroup,
	},
	SnapHTTPProxyKey: {
		Description: "The HTTP proxy value for installing snaps",
		Type:        configschema.Tstring,
		Group:       configschema.EnvironGroup,
	},
	SnapHTTPSProxyKey: {
		Description: "The HTTPS proxy value for installing snaps",
		Type:        configschema.Tstring,
		Group:       configschema.EnvironGroup,
	},
	SnapStoreProxyKey: {
		Description: "The snap store proxy for installing snaps",
		Type:        configschema.Tstring,
		Group:       configschema.EnvironGroup,
	},
	SnapStoreAssertionsKey: {
		Description: "The assertions for the defined snap store proxy",
		Type:        configschema.Tstring,
		Group:       configschema.EnvironGroup,
	},
	SnapStoreProxyURLKey: {
		Description: "The URL for the defined snap store proxy",
		Type:        configschema.Tstring,
		Group:       configschema.EnvironGroup,
	},
	ImageMetadataURLKey: {
		Description: "The URL at which the metadata used to locate OS image ids is located",
		Type:        configschema.Tstring,
		Group:       configschema.EnvironGroup,
	},
	ImageStreamKey: {
		Description: `The simplestreams stream used to identify which image ids to search when starting an instance.`,
		Type:        configschema.Tstring,
		Group:       configschema.EnvironGroup,
	},
	ImageMetadataDefaultsDisabledKey: {
		Description: `Whether default simplestreams sources are used for image metadata.`,
		Type:        configschema.Tbool,
		Group:       configschema.EnvironGroup,
	},
	ContainerImageMetadataURLKey: {
		Description: "The URL at which the metadata used to locate container OS image ids is located",
		Type:        configschema.Tstring,
		Group:       configschema.EnvironGroup,
	},
	ContainerImageStreamKey: {
		Description: `The simplestreams stream used to identify which image ids to search when starting a container.`,
		Type:        configschema.Tstring,
		Group:       configschema.EnvironGroup,
	},
	ContainerImageMetadataDefaultsDisabledKey: {
		Description: `Whether default simplestreams sources are used for image metadata with containers.`,
		Type:        configschema.Tbool,
		Group:       configschema.EnvironGroup,
	},
	"logging-config": {
		Description: `The configuration string to use when configuring Juju agent logging (see http://godoc.org/github.com/juju/loggo#ParseConfigurationString for details)`,
		Type:        configschema.Tstring,
		Group:       configschema.EnvironGroup,
	},
	NameKey: {
		Description: "The name of the current model",
		Type:        configschema.Tstring,
		Mandatory:   true,
		Immutable:   true,
		Group:       configschema.EnvironGroup,
	},
	ProvisionerHarvestModeKey: {
		// default: destroyed, but also depends on current setting of ProvisionerSafeModeKey
		Description: "What to do with unknown machines (default destroyed)",
		Type:        configschema.Tstring,
		Values:      []interface{}{"all", "none", "unknown", "destroyed"},
		Group:       configschema.EnvironGroup,
	},
	NumProvisionWorkersKey: {
		Description: "The number of provisioning workers to use per model",
		Type:        configschema.Tint,
		Group:       configschema.EnvironGroup,
	},
	NumContainerProvisionWorkersKey: {
		Description: "The number of container provisioning workers to use per machine",
		Type:        configschema.Tint,
		Group:       configschema.EnvironGroup,
	},
	"proxy-ssh": {
		// default: true
		Description: `Whether SSH commands should be proxied through the API server`,
		Type:        configschema.Tbool,
		Group:       configschema.EnvironGroup,
	},
	ResourceTagsKey: {
		Description: "resource tags",
		Type:        configschema.Tattrs,
		Group:       configschema.EnvironGroup,
	},
	SSLHostnameVerificationKey: {
		Description: "Whether SSL hostname verification is enabled (default true)",
		Type:        configschema.Tbool,
		Group:       configschema.EnvironGroup,
	},
	StorageDefaultBlockSourceKey: {
		Description: "The default block storage source for the model",
		Type:        configschema.Tstring,
		Group:       configschema.EnvironGroup,
	},
	StorageDefaultFilesystemSourceKey: {
		Description: "The default filesystem storage source for the model",
		Type:        configschema.Tstring,
		Group:       configschema.EnvironGroup,
	},
	TestModeKey: {
		Description: `Whether the model is intended for testing.
If true, accessing the charm store does not affect statistical
data of the store. (default false)`,
		Type:  configschema.Tbool,
		Group: configschema.EnvironGroup,
	},
	DisableTelemetryKey: {
		Description: `Disable telemetry reporting of model information`,
		Type:        configschema.Tbool,
		Group:       configschema.EnvironGroup,
	},
	ModeKey: {
		Description: `Mode is a comma-separated list which sets the
mode the model should run in. So far only one is implemented
- If 'requires-prompts' is present, clients will ask for confirmation before removing
potentially valuable resources.
(default "")`,
		Type:  configschema.Tstring,
		Group: configschema.EnvironGroup,
	},
	SSHAllowKey: {
		Description: `SSH allowlist is a comma-separated list of CIDRs from
which machines in this model will accept connections to the SSH service.
Currently only the aws & openstack providers support ssh-allow`,
		Type:  configschema.Tstring,
		Group: configschema.EnvironGroup,
	},
	SAASIngressAllowKey: {
		Description: `Application-offer ingress allowlist is a comma-separated list of
CIDRs specifying what ingress can be applied to offers in this model.`,
		Type:  configschema.Tstring,
		Group: configschema.EnvironGroup,
	},
	TypeKey: {
		Description: "Type of model, e.g. local, ec2",
		Type:        configschema.Tstring,
		Mandatory:   true,
		Immutable:   true,
		Group:       configschema.EnvironGroup,
	},
	UUIDKey: {
		Description: "The UUID of the model",
		Type:        configschema.Tstring,
		Group:       configschema.JujuGroup,
		Immutable:   true,
	},
	AutomaticallyRetryHooks: {
		Description: "Determines whether the uniter should automatically retry failed hooks",
		Type:        configschema.Tbool,
		Group:       configschema.EnvironGroup,
	},
	TransmitVendorMetricsKey: {
		Description: "Determines whether metrics declared by charms deployed into this model are sent for anonymized aggregate analytics",
		Type:        configschema.Tbool,
		Group:       configschema.EnvironGroup,
	},
	NetBondReconfigureDelayKey: {
		Description: "The amount of time in seconds to sleep between ifdown and ifup when bridging",
		Type:        configschema.Tint,
		Group:       configschema.EnvironGroup,
	},
	ContainerNetworkingMethodKey: {
		Description: `Method of container networking setup - one of "provider", "local", or "" (auto-configure).`,
		Type:        configschema.Tstring,
		Group:       configschema.EnvironGroup,
	},
	MaxActionResultsAge: {
		Description: "The maximum age for action entries before they are pruned, in human-readable time format",
		Type:        configschema.Tstring,
		Group:       configschema.EnvironGroup,
	},
	MaxActionResultsSize: {
		Description: "The maximum size for the action collection, in human-readable memory format",
		Type:        configschema.Tstring,
		Group:       configschema.EnvironGroup,
	},
	UpdateStatusHookInterval: {
		Description: "How often to run the charm update-status hook, in human-readable time format (default 5m, range 1-60m)",
		Type:        configschema.Tstring,
		Group:       configschema.EnvironGroup,
	},
	EgressSubnets: {
		Description: "Source address(es) for traffic originating from this model",
		Type:        configschema.Tstring,
		Group:       configschema.EnvironGroup,
	},
	CloudInitUserDataKey: {
		Description: "Cloud-init user-data (in yaml format) to be added to userdata for new machines created in this model",
		Type:        configschema.Tstring,
		Group:       configschema.EnvironGroup,
	},
	ContainerInheritPropertiesKey: {
		Description: "List of properties to be copied from the host machine to new containers created in this model (comma-separated)",
		Type:        configschema.Tstring,
		Group:       configschema.EnvironGroup,
	},
	BackupDirKey: {
		Description: "Directory used to store the backup working directory",
		Type:        configschema.Tstring,
		Group:       configschema.EnvironGroup,
	},
	DefaultSpaceKey: {
		Description: "The default network space used for application endpoints in this model",
		Type:        configschema.Tstring,
		Group:       configschema.EnvironGroup,
	},
	LXDSnapChannel: {
		Description: "The channel to use when installing LXD from a snap (cosmic and later)",
		Type:        configschema.Tstring,
		Group:       configschema.EnvironGroup,
	},
	CharmHubURLKey: {
		Description: `The url for CharmHub API calls`,
		Type:        configschema.Tstring,
		Group:       configschema.EnvironGroup,
	},
}
