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
			return nil, errors.Errorf(
				"config field %q clashes with controller config",
				name,
			)
		}
		fields[name] = field
	}
	for name, field := range extra {
		if controller.ControllerOnlyAttribute(name) {
			return nil, errors.Errorf(
				"config field %q clashes with controller config",
				name,
			)
		}
		if _, ok := fields[name]; ok {
			return nil, errors.Errorf(
				"config field %q clashes with global config",
				name,
			)
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
		Documentation: `
The agent-stream key specifies the “stream” to use when a Juju agent is to be
installed or upgraded. This setting reflects the general stability of the
software and defaults to ‘released’, indicating that only the latest stable
version is to be used.

To run the upcoming stable release (before it has passed the normal QA process)
you can set:

	agent-stream: proposed

For testing purposes, you can use the latest unstable version by setting:

	agent-stream: devel

The agent-version option specifies a “patch version” for the agent that is to be
installed on a new controller relative to the Juju client’s current major.minor
version (Juju uses a major.minor.patch numbering scheme).

For example, Juju 3.6.2 means major version 3, minor version 6, and patch
version 2. On a client system with this release of Juju installed, the machine
agent’s version for a newly-created controller would be the same. To specify a
patch version of 1 (instead of 2), the following would be run:

	juju bootstrap aws --agent-version='3.6.1'

If a patch version is available that is greater than that of the client then it
can be targeted in this way:

	juju bootstrap aws --auto-upgrade
`,
		Type:  configschema.Tstring,
		Group: configschema.EnvironGroup,
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
		Description: "The APT mirror for the model",
		Documentation: `
The APT packaging system is used to install and upgrade software on machines
provisioned in the model, and many charms also use APT to install software for
the applications they deploy. It is possible to set a specific mirror for the
APT packages to use, by setting ‘apt-mirror’:

	juju model-config apt-mirror=http://archive.ubuntu.com/ubuntu/

To restore the default behaviour you would run:

	juju model-config --reset apt-mirror

The apt-mirror option is often used to point to a local mirror.
`,
		Type:  configschema.Tstring,
		Group: configschema.EnvironGroup,
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
		Description: `Whether the provider should control networks (on MAAS models, set to true for MAAS to control networks`,
		Documentation: `
This key can only be used with MAAS models and should otherwise be set to
‘false’ (default) unless you want to take over network control from Juju because
you have unique and well-defined needs. Setting this to ‘true’ with MAAS gives
you the same behaviour with containers as you already have with other providers:
one machine-local address on a single network interface, bridged to the default
bridge.
`,
		Type:  configschema.Tbool,
		Group: configschema.EnvironGroup,
	},
	IgnoreMachineAddresses: {
		Description: "Whether the machine worker should discover machine addresses on startup",
		Type:        configschema.Tbool,
		Group:       configschema.EnvironGroup,
	},
	EnableOSRefreshUpdateKey: {
		Description: `Whether newly provisioned instances should run their respective OS's update capability.`,
		Documentation: `
When Juju provisions a machine, its default behaviour is to upgrade existing
packages to their latest version. If your OS images are fresh and/or your
deployed applications do not require the latest package versions, you can
disable upgrades in order to provision machines faster.

Two boolean configuration options are available to disable APT updates and
upgrades: enable-os-refresh-update (apt update) and enable-os-upgrade (apt
upgrade), respectively.

	enable-os-refresh-update: false
	enable-os-upgrade: false

You may also want to just update the package list to ensure a charm has the
latest software available to it by disabling upgrades but enabling updates.
`,
		Type:  configschema.Tbool,
		Group: configschema.EnvironGroup,
	},
	EnableOSUpgradeKey: {
		Description: `Whether newly provisioned instances should run their respective OS's upgrade capability.`,
		Documentation: `
When Juju provisions a machine, its default behaviour is to upgrade existing
packages to their latest version. If your OS images are fresh and/or your
deployed applications do not require the latest package versions, you can
disable upgrades in order to provision machines faster.

Two Boolean configuration options are available to disable APT updates and
upgrades: enable-os-refresh-update (apt update) and enable-os-upgrade (apt
upgrade), respectively.

	enable-os-refresh-update: false
	enable-os-upgrade: false

You may also want to just update the package list to ensure a charm has the
latest software available to it by disabling upgrades but enabling updates.
`,
		Type:  configschema.Tbool,
		Group: configschema.EnvironGroup,
	},
	ExtraInfoKey: {
		Description: "Arbitrary user specified string data that is stored against the model.",
		Type:        configschema.Tstring,
		Group:       configschema.EnvironGroup,
	},
	"firewall-mode": {
		Description: `The mode to use for network firewalling.`,
		Documentation: `
- 'instance' requests the use of an individual firewall per instance.

- 'global' uses a single firewall for all instances (access
for a network port is enabled to one instance if any instance requires
that port).

- 'none' requests that no firewalling should be performed
inside the model. It's useful for clouds without support for either
global or per instance security groups.`,
		Type:      configschema.Tstring,
		Values:    []interface{}{FwInstance, FwGlobal, FwNone},
		Immutable: true,
		Group:     configschema.EnvironGroup,
	},
	FTPProxyKey: {
		Description: "The FTP proxy value to configure on instances, in the `FTP_PROXY` environment variable",
		Type:        configschema.Tstring,
		Group:       configschema.EnvironGroup,
	},
	HTTPProxyKey: {
		Description: "The HTTP proxy value to configure on instances, in the `HTTP_PROXY` environment variable",
		Type:        configschema.Tstring,
		Group:       configschema.EnvironGroup,
	},
	HTTPSProxyKey: {
		Description: "The HTTPS proxy value to configure on instances, in the `HTTPS_PROXY` environment variable",
		Type:        configschema.Tstring,
		Group:       configschema.EnvironGroup,
	},
	NoProxyKey: {
		Description: "List of domain addresses not to be proxied (comma-separated)",
		Type:        configschema.Tstring,
		Group:       configschema.EnvironGroup,
	},
	JujuFTPProxyKey: {
		Description: "The FTP proxy value to pass to charms in the `JUJU_CHARM_FTP_PROXY` environment variable",
		Type:        configschema.Tstring,
		Group:       configschema.EnvironGroup,
	},
	JujuHTTPProxyKey: {
		Description: "The HTTP proxy value to pass to charms in the `JUJU_CHARM_HTTP_PROXY` environment variable",
		Type:        configschema.Tstring,
		Group:       configschema.EnvironGroup,
	},
	JujuHTTPSProxyKey: {
		Description: "The HTTPS proxy value to pass to charms in the `JUJU_CHARM_HTTPS_PROXY` environment variable",
		Type:        configschema.Tstring,
		Group:       configschema.EnvironGroup,
	},
	JujuNoProxyKey: {
		Description: "List of domain addresses not to be proxied (comma-separated), may contain CIDRs. Passed to charms in the `JUJU_CHARM_NO_PROXY` environment variable",
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
		Documentation: `
Juju, by default, uses the slow-changing ‘released’ images when provisioning
machines. However, the image-stream option can be set to ‘daily’ to use more
up-to-date images, thus shortening the time it takes to perform APT package
upgrades.
`,
		Type:  configschema.Tstring,
		Group: configschema.EnvironGroup,
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
		Description: `The configuration string to use when configuring Juju agent logging (see [this link](https://pkg.go.dev/github.com/juju/loggo#ParseConfigString) for details)`,
		Documentation: "The logging config can be set to a (list of semicolon-separated)\n" +
			"`<filter>=<verbosity level>` pairs, where `<filter>` can be any of the following:\n" +
			" - `<root>` - matches all machine agent logs\n" +
			" - `unit` - matches all unit agent logs\n" +
			" - a module name, e.g. `juju.worker.apiserver`\n" +
			"   A module represents a single component of Juju, e.g. a worker. Generally,\n" +
			"   modules correspond one-to-one with Go packages in the Juju source tree. The\n" +
			"   module name is the value passed to `loggo.GetLogger` or\n" +
			"   `loggo.GetLoggerWithLabels`.\n" +
			"\n" +
			"   Modules have a nested tree structure - for example, the `juju.api` module\n" +
			"   includes submodules `juju.api.application`, `juju.api.cloud`, etc. `<root>` is the\n" +
			"   root of this module tree.\n" +
			"\n" +
			" - a label, e.g. `#charmhub`\n" +
			"    Labels cut across the module tree, grouping various modules which deal with\n" +
			"    a certain feature or information flow. For example, the `#charmhub` label\n" +
			"    includes all modules involved in making a request to Charmhub.\n" +
			"\n" +
			"The currently supported labels are:\n" +
			"| Label | Description |\n" +
			"|-|-|\n" +
			"| `#http` | HTTP requests |\n" +
			"| `#metrics` | Metric outputs - use as a fallback when Prometheus isn't available |\n" +
			"| `#charmhub` | Charmhub client and callers. |\n" +
			"| `#cmr` | Cross model relations |\n" +
			"| `#cmr-auth` | Authentication for cross model relations |\n" +
			"| `#secrets` | Juju secrets |\n" +
			"\n" +
			"and where <verbosity level> can be, in decreasing order of severity:\n" +
			"\n" +
			"| Level | Description |\n" +
			"|-|-|\n" +
			"| `CRITICAL` | Indicates a severe failure which could bring down the system. |\n" +
			"| `ERROR` | Indicates failure to complete a routine operation.\n" +
			"| `WARNING` | Indicates something is not as expected, but this is not necessarily going to cause an error.\n" +
			"| `INFO` | A regular log message intended for the user.\n" +
			"| `DEBUG` | Information intended to assist developers in debugging.\n" +
			"| `TRACE` | The lowest level - includes the full details of input args, return values, HTTP requests sent/received, etc. |\n" +
			"\n" +
			"When you set `logging-config` to `module=level`, then Juju saves that module's logs\n" +
			"for the given severity level **and above.** For example, setting `logging-config`\n" +
			"to `juju.worker.uniter=WARNING` will capture all `CRITICAL`, `ERROR` and `WARNING` logs\n" +
			"for the uniter, but discard logs for lower severity levels (`INFO`, `DEBUG`, `TRACE`).\n" +
			"\n" +
			"**Examples:**\n" +
			"\n" +
			"To collect debug logs for the dbaccessor worker:\n" +
			"\n" +
			"	juju model-config -m controller logging-config=\"juju.worker.dbaccessor=DEBUG\"\n" +
			"\n" +
			"To collect debug logs for the mysql/0 unit:\n" +
			"\n" +
			"	juju model-config -m foo logging-config=\"unit.mysql/0=DEBUG\"\n" +
			"\n" +
			"To collect trace logs for Charmhub requests:\n" +
			"\n" +
			"	juju model-config -m controller logging-config=\"#charmhub=TRACE\"\n" +
			"\n" +
			"To see what API requests are being made:\n" +
			"\n" +
			"	juju model-config -m controller logging-config=\"juju.apiserver=DEBUG\"\n" +
			"\n" +
			"To view details about each API request:\n" +
			"\n" +
			"	juju model-config -m controller logging-config=\"juju.apiserver=TRACE\"\n",
		Type:  configschema.Tstring,
		Group: configschema.EnvironGroup,
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
		Description: `What to do with unknown machines (default destroyed)`,
		Documentation: `
Juju keeps state on the running model and it can harvest (remove) machines which it deems are no longer required. This can help reduce running costs and keep the model tidy. Harvesting is guided by what "harvesting mode" has been set.

A Juju machine can be in one of four states:

-   **Alive:** The machine is running and being used.
-   **Dying:** The machine is in the process of being terminated by Juju, but hasn't yet finished.
-   **Dead:** The machine has been successfully brought down by Juju, but is still being tracked for removal.
-   **Unknown:** The machine exists, but Juju knows nothing about it.

Juju can be in one of several harvesting modes, in order of most conservative to most aggressive:

-   **none:** Machines will never be harvested. This is a good choice if machines are managed via a process outside of Juju.
-   **destroyed:** Machines will be harvested if i) Juju "knows" about them and

ii) they are 'Dead'. - **unknown:** Machines will be harvested if Juju does not "know" about them ('Unknown' state). Use with caution in a mixed environment or one which may contain multiple instances of Juju. - **all:** Machines will be harvested if Juju considers them to be 'destroyed' or 'unknown'.

The default mode is **destroyed**.

Below, the harvest mode key for the current model is set to 'none':

	juju model-config provisioner-harvest-mode=none

`,
		Type:   configschema.Tstring,
		Values: []interface{}{"all", "none", "unknown", "destroyed"},
		Group:  configschema.EnvironGroup,
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
		Description: `Determines whether the uniter should automatically retry failed hooks`,
		Documentation: `
Juju retries failed hooks automatically using an exponential backoff algorithm.
They will be retried after 5, 10, 20, 40 seconds up to a period of 5 minutes,
and then every 5 minutes. The logic behind this is that some hook errors are
caused by timing issues or the temporary unavailability of other applications -
automatic retry enables the Juju model to heal itself without troubling the
user.

However, in some circumstances, such as debugging charms, this behaviour can be
distracting and unwelcome. For this reason, it is possible to set the
automatically-retry-hooks option to ‘false’ to disable this behaviour. In this
case, users will have to manually retry any hook which fails, using the command
above, as with earlier versions of Juju.

Even with the automatic retry enabled, it is still possible to use retry
manually using:

	juju resolved unit-name/# 
`,
		Type:  configschema.Tbool,
		Group: configschema.EnvironGroup,
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
		Description: `Cloud-init user-data (in yaml format) to be added to userdata for new machines created in this model`,
		Documentation: `
The cloudinit-userdata allows the user to provide additional cloudinit data to
be included in the cloudinit data created by Juju.

Specifying a key will overwrite what juju puts in the cloudinit file with the
following caveats:

 1. users and bootcmd keys will cause an error
 2. The packages key will be appended to the packages listed by juju
 3. The runcmds key will cause an error. You can specify preruncmd and
    postruncmd keys to prepend and append the runcmd created by Juju.

**Use cases**

 - setting a default locale for deployments that wish to use their own locale settings
 - adding custom CA certificates for models that are sitting behind an HTTPS proxy
 - adding a private apt mirror to enable private packages to be installed
 - add SSH fingerprints to a deny list to prevent them from being printed to the console for security-focused deployments

**Background**

Juju uses cloud-init to customise instances once they have been provisioned by
the cloud. The cloudinit-userdata model configuration setting (model config)
allows you to tweak what happens to machines when they are created up via the
“user data” feature.

From the website:

> Cloud images are operating system templates and every instance starts out as
  an identical clone of every other instance. It is the user data that gives
  every cloud instance its personality and cloud-init is the tool that applies
  user data to your instances automatically.

**How to provide custom user data to cloudinit**

Create a file, cloudinit-userdata.yaml, which starts with the cloudinit-userdata
key and data you wish to include in the cloudinit file. Note: juju reads the
value as a string, though formatted as YAML.

Template cloudinit-userdata.yaml:

	cloudinit-userdata: |
		<key>: <value>
		<key>: <value>

Provide the path your file to the model-config command:

	juju model-config --file cloudinit-userdata.yaml

**How to read the current setting**

To read the current value, provide the cloudinit-userdata key to the
model-config command as a command-line parameter. Adding the --format yaml
option ensures that it is properly formatted.

	juju model-config cloudinit-userdata --format yaml

Sample output:

	cloudinit-userdata: |
	  packages:
		- 'python-keystoneclient'
		- 'python-glanceclient'

**How to clear the current custom user data**

Use the --reset option to the model-config command to clear anything that has
been previously set.

	juju model-config --reset cloudinit-userdata

**Known issues**

- custom cloudinit-userdata must be passed via file, not as options on the command
line (like the config command)
`,
		Type:  configschema.Tstring,
		Group: configschema.EnvironGroup,
	},
	ContainerInheritPropertiesKey: {
		Description: `List of properties to be copied from the host machine to new containers created in this model (comma-separated)`,
		Documentation: `
The container-inherit-properties key allows for a limited set of parameters
enabled on a Juju machine to be inherited by any hosted containers (KVM guests
or LXD containers). The machine and container must be running the same series.

This key is only supported by the MAAS provider.

The parameters are:
 - apt-primary
 - apt-security
 - apt-sources
 - ca-certs

For MAAS v.2.5 or greater the parameters are:
 - apt-sources
 - ca-certs

For example:

	juju model-config container-inherit-properties="ca-certs, apt-sources"
`,
		Type:  configschema.Tstring,
		Group: configschema.EnvironGroup,
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
