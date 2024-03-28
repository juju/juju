// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config

import (
	"github.com/juju/errors"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/controller"
)

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
	DefaultSpaceKey: {
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
	SecretBackendKey: {
		Description: `The name of the secret store backend. (default "auto")`,
		Type:        environschema.Tstring,
		Group:       environschema.EnvironGroup,
	},
}
