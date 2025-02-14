(list-of-model-configuration-keys)=
# List of model configuration keys


This document gives a list of all the configuration keys that can be applied to a Juju model.
(model-config-agent-metadata-url)=
## `agent-metadata-url`

URL of private stream.

**Default value:** `""`

**Type:** string


(model-config-agent-stream)=
## `agent-stream`

Version of Juju to use for deploy/upgrades.

**Default value:** `released`

**Type:** string

**Description:**


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



(model-config-agent-version)=
## `agent-version`

*Note: This value is set by Juju.*

The desired Juju agent version to use.

**Default value:** `""`

**Type:** string


(model-config-apt-ftp-proxy)=
## `apt-ftp-proxy`

The APT FTP proxy for the model.

**Default value:** `""`

**Type:** string


(model-config-apt-http-proxy)=
## `apt-http-proxy`

The APT HTTP proxy for the model.

**Default value:** `""`

**Type:** string


(model-config-apt-https-proxy)=
## `apt-https-proxy`

The APT HTTPS proxy for the model.

**Default value:** `""`

**Type:** string


(model-config-apt-mirror)=
## `apt-mirror`

The APT mirror for the model.

**Default value:** `""`

**Type:** string

**Description:**


The APT packaging system is used to install and upgrade software on machines
provisioned in the model, and many charms also use APT to install software for
the applications they deploy. It is possible to set a specific mirror for the
APT packages to use, by setting ‘apt-mirror’:

	juju model-config apt-mirror=http://archive.ubuntu.com/ubuntu/

To restore the default behaviour you would run:

	juju model-config --reset apt-mirror

The apt-mirror option is often used to point to a local mirror.



(model-config-apt-no-proxy)=
## `apt-no-proxy`

List of domain addresses not to be proxied for APT (comma-separated).

**Default value:** `""`

**Type:** string


(model-config-automatically-retry-hooks)=
## `automatically-retry-hooks`

Determines whether the uniter should automatically retry failed hooks.

**Default value:** `true`

**Type:** bool

**Description:**


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



(model-config-backup-dir)=
## `backup-dir`

Directory used to store the backup working directory.

**Default value:** `""`

**Type:** string


(model-config-charmhub-url)=
## `charmhub-url`

The url for CharmHub API calls.

**Default value:** `https://api.charmhub.io`

**Type:** string


(model-config-cloudinit-userdata)=
## `cloudinit-userdata`

Cloud-init user-data (in yaml format) to be added to userdata for new machines created in this model.

**Default value:** `""`

**Type:** string

**Description:**


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



(model-config-container-image-metadata-defaults-disabled)=
## `container-image-metadata-defaults-disabled`

Whether default simplestreams sources are used for image metadata with containers.

**Default value:** `false`

**Type:** bool


(model-config-container-image-metadata-url)=
## `container-image-metadata-url`

The URL at which the metadata used to locate container OS image ids is located.

**Default value:** `""`

**Type:** string


(model-config-container-image-stream)=
## `container-image-stream`

The simplestreams stream used to identify which image ids to search when starting a container.

**Default value:** `released`

**Type:** string


(model-config-container-inherit-properties)=
## `container-inherit-properties`

List of properties to be copied from the host machine to new containers created in this model (comma-separated).

**Default value:** `""`

**Type:** string

**Description:**


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



(model-config-container-networking-method)=
## `container-networking-method`

Method of container networking setup - one of "provider", "local", or "" (auto-configure).

**Default value:** `""`

**Type:** string


(model-config-default-base)=
## `default-base`

The default base image to use for deploying charms, will act like --base when deploying charms.

**Default value:** `""`

**Type:** string


(model-config-default-space)=
## `default-space`

The default network space used for application endpoints in this model.

**Default value:** `""`

**Type:** string


(model-config-development)=
## `development`

Whether the model is in development mode.

**Default value:** `false`

**Type:** bool


(model-config-disable-network-management)=
## `disable-network-management`

Whether the provider should control networks (on MAAS models, set to true for MAAS to control networks.

**Default value:** `false`

**Type:** bool

**Description:**


This key can only be used with MAAS models and should otherwise be set to
‘false’ (default) unless you want to take over network control from Juju because
you have unique and well-defined needs. Setting this to ‘true’ with MAAS gives
you the same behaviour with containers as you already have with other providers:
one machine-local address on a single network interface, bridged to the default
bridge.



(model-config-disable-telemetry)=
## `disable-telemetry`

Disable telemetry reporting of model information.

**Default value:** `false`

**Type:** bool


(model-config-egress-subnets)=
## `egress-subnets`

Source address(es) for traffic originating from this model.

**Default value:** `""`

**Type:** string


(model-config-enable-os-refresh-update)=
## `enable-os-refresh-update`

Whether newly provisioned instances should run their respective OS's update capability.

**Default value:** `true`

**Type:** bool

**Description:**


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



(model-config-enable-os-upgrade)=
## `enable-os-upgrade`

Whether newly provisioned instances should run their respective OS's upgrade capability.

**Default value:** `true`

**Type:** bool

**Description:**


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



(model-config-extra-info)=
## `extra-info`

Arbitrary user specified string data that is stored against the model.

**Default value:** `""`

**Type:** string


(model-config-firewall-mode)=
## `firewall-mode`

*Note: This value cannot be changed after model creation.* 

The mode to use for network firewalling.

**Default value:** `instance`

**Type:** string

**Valid values:** `instance`, `global`, `none`

**Description:**


- 'instance' requests the use of an individual firewall per instance.

- 'global' uses a single firewall for all instances (access
for a network port is enabled to one instance if any instance requires
that port).

- 'none' requests that no firewalling should be performed
inside the model. It's useful for clouds without support for either
global or per instance security groups.


(model-config-ftp-proxy)=
## `ftp-proxy`

The FTP proxy value to configure on instances, in the `FTP_PROXY` environment variable.

**Default value:** `""`

**Type:** string


(model-config-http-proxy)=
## `http-proxy`

The HTTP proxy value to configure on instances, in the `HTTP_PROXY` environment variable.

**Default value:** `""`

**Type:** string


(model-config-https-proxy)=
## `https-proxy`

The HTTPS proxy value to configure on instances, in the `HTTPS_PROXY` environment variable.

**Default value:** `""`

**Type:** string


(model-config-ignore-machine-addresses)=
## `ignore-machine-addresses`

Whether the machine worker should discover machine addresses on startup.

**Default value:** `false`

**Type:** bool


(model-config-image-metadata-defaults-disabled)=
## `image-metadata-defaults-disabled`

Whether default simplestreams sources are used for image metadata.

**Default value:** `false`

**Type:** bool


(model-config-image-metadata-url)=
## `image-metadata-url`

The URL at which the metadata used to locate OS image ids is located.

**Default value:** `""`

**Type:** string


(model-config-image-stream)=
## `image-stream`

The simplestreams stream used to identify which image ids to search when starting an instance.

**Default value:** `released`

**Type:** string

**Description:**


Juju, by default, uses the slow-changing ‘released’ images when provisioning
machines. However, the image-stream option can be set to ‘daily’ to use more
up-to-date images, thus shortening the time it takes to perform APT package
upgrades.



(model-config-juju-ftp-proxy)=
## `juju-ftp-proxy`

The FTP proxy value to pass to charms in the `JUJU_CHARM_FTP_PROXY` environment variable.

**Default value:** `""`

**Type:** string


(model-config-juju-http-proxy)=
## `juju-http-proxy`

The HTTP proxy value to pass to charms in the `JUJU_CHARM_HTTP_PROXY` environment variable.

**Default value:** `""`

**Type:** string


(model-config-juju-https-proxy)=
## `juju-https-proxy`

The HTTPS proxy value to pass to charms in the `JUJU_CHARM_HTTPS_PROXY` environment variable.

**Default value:** `""`

**Type:** string


(model-config-juju-no-proxy)=
## `juju-no-proxy`

List of domain addresses not to be proxied (comma-separated), may contain CIDRs. Passed to charms in the `JUJU_CHARM_NO_PROXY` environment variable.

**Default value:** `127.0.0.1,localhost,::1`

**Type:** string


(model-config-logging-config)=
## `logging-config`

The configuration string to use when configuring Juju agent logging.

**Default value:** `""`

**Type:** string

**Description:**

The logging config can be set to a (list of semicolon-separated)
`<filter>=<verbosity level>` pairs, where `<filter>` can be any of the following:
 - `<root>` - matches all machine agent logs
 - `unit` - matches all unit agent logs
 - a module name, e.g. `juju.worker.apiserver`
   A module represents a single component of Juju, e.g. a worker. Generally,
   modules correspond one-to-one with Go packages in the Juju source tree. The
   module name is the value passed to `loggo.GetLogger` or
   `loggo.GetLoggerWithLabels`.

   Modules have a nested tree structure - for example, the `juju.api` module
   includes submodules `juju.api.application`, `juju.api.cloud`, etc. `<root>` is the
   root of this module tree.

 - a label, e.g. `#charmhub`
    Labels cut across the module tree, grouping various modules which deal with
    a certain feature or information flow. For example, the `#charmhub` label
    includes all modules involved in making a request to Charmhub.

The currently supported labels are:
| Label | Description |
|-|-|
| `#http` | HTTP requests |
| `#metrics` | Metric outputs - use as a fallback when Prometheus isn't available |
| `#charmhub` | Charmhub client and callers. |
| `#cmr` | Cross model relations |
| `#cmr-auth` | Authentication for cross model relations |
| `#secrets` | Juju secrets |

and where <verbosity level> can be, in decreasing order of severity:

| Level | Description |
|-|-|
| `CRITICAL` | Indicates a severe failure which could bring down the system. |
| `ERROR` | Indicates failure to complete a routine operation.
| `WARNING` | Indicates something is not as expected, but this is not necessarily going to cause an error.
| `INFO` | A regular log message intended for the user.
| `DEBUG` | Information intended to assist developers in debugging.
| `TRACE` | The lowest level - includes the full details of input args, return values, HTTP requests sent/received, etc. |

When you set `logging-config` to `module=level`, then Juju saves that module's logs
for the given severity level **and above.** For example, setting `logging-config`
to `juju.worker.uniter=WARNING` will capture all `CRITICAL`, `ERROR` and `WARNING` logs
for the uniter, but discard logs for lower severity levels (`INFO`, `DEBUG`, `TRACE`).

**Examples:**

To collect debug logs for the dbaccessor worker:

	juju model-config -m controller logging-config="juju.worker.dbaccessor=DEBUG"

To collect debug logs for the mysql/0 unit:

	juju model-config -m foo logging-config="unit.mysql/0=DEBUG"

To collect trace logs for Charmhub requests:

	juju model-config -m controller logging-config="#charmhub=TRACE"

To see what API requests are being made:

	juju model-config -m controller logging-config="juju.apiserver=DEBUG"

To view details about each API request:

	juju model-config -m controller logging-config="juju.apiserver=TRACE"



(model-config-lxd-snap-channel)=
## `lxd-snap-channel`

The channel to use when installing LXD from a snap (cosmic and later).

**Default value:** `5.0/stable`

**Type:** string


(model-config-max-action-results-age)=
## `max-action-results-age`

The maximum age for action entries before they are pruned, in human-readable time format.

**Default value:** `336h`

**Type:** string


(model-config-max-action-results-size)=
## `max-action-results-size`

The maximum size for the action collection, in human-readable memory format.

**Default value:** `5G`

**Type:** string


(model-config-mode)=
## `mode`

Mode is a comma-separated list which sets the
mode the model should run in. So far only one is implemented
- If 'requires-prompts' is present, clients will ask for confirmation before removing
potentially valuable resources.
(default "").

**Default value:** `requires-prompts`

**Type:** string


(model-config-name)=
## `name`

*Note: This value cannot be changed after model creation.* 

*Note: This value must be set.* 

The name of the current model.

**Default value:** `""`

**Type:** string


(model-config-net-bond-reconfigure-delay)=
## `net-bond-reconfigure-delay`

The amount of time in seconds to sleep between ifdown and ifup when bridging.

**Default value:** `17`

**Type:** int


(model-config-no-proxy)=
## `no-proxy`

List of domain addresses not to be proxied (comma-separated).

**Default value:** `127.0.0.1,localhost,::1`

**Type:** string


(model-config-num-container-provision-workers)=
## `num-container-provision-workers`

The number of container provisioning workers to use per machine.

**Default value:** `4`

**Type:** int


(model-config-num-provision-workers)=
## `num-provision-workers`

The number of provisioning workers to use per model.

**Default value:** `16`

**Type:** int


(model-config-provisioner-harvest-mode)=
## `provisioner-harvest-mode`

What to do with unknown machines (default destroyed).

**Default value:** `destroyed`

**Type:** string

**Valid values:** `all`, `none`, `unknown`, `destroyed`

**Description:**


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




(model-config-proxy-ssh)=
## `proxy-ssh`

Whether SSH commands should be proxied through the API server.

**Default value:** `false`

**Type:** bool


(model-config-resource-tags)=
## `resource-tags`

resource tags.

**Default value:** `""`

**Type:** attrs


(model-config-saas-ingress-allow)=
## `saas-ingress-allow`

Application-offer ingress allowlist is a comma-separated list of
CIDRs specifying what ingress can be applied to offers in this model.

**Default value:** `0.0.0.0/0,::/0`

**Type:** string


(model-config-snap-http-proxy)=
## `snap-http-proxy`

The HTTP proxy value for installing snaps.

**Default value:** `""`

**Type:** string


(model-config-snap-https-proxy)=
## `snap-https-proxy`

The HTTPS proxy value for installing snaps.

**Default value:** `""`

**Type:** string


(model-config-snap-store-assertions)=
## `snap-store-assertions`

The assertions for the defined snap store proxy.

**Default value:** `""`

**Type:** string


(model-config-snap-store-proxy)=
## `snap-store-proxy`

The snap store proxy for installing snaps.

**Default value:** `""`

**Type:** string


(model-config-snap-store-proxy-url)=
## `snap-store-proxy-url`

The URL for the defined snap store proxy.

**Default value:** `""`

**Type:** string


(model-config-ssh-allow)=
## `ssh-allow`

SSH allowlist is a comma-separated list of CIDRs from
which machines in this model will accept connections to the SSH service.
Currently only the aws & openstack providers support ssh-allow.

**Default value:** `0.0.0.0/0,::/0`

**Type:** string


(model-config-ssl-hostname-verification)=
## `ssl-hostname-verification`

Whether SSL hostname verification is enabled (default true).

**Default value:** `true`

**Type:** bool


(model-config-storage-default-block-source)=
## `storage-default-block-source`

The default block storage source for the model.

**Default value:** `""`

**Type:** string


(model-config-storage-default-filesystem-source)=
## `storage-default-filesystem-source`

The default filesystem storage source for the model.

**Default value:** `""`

**Type:** string


(model-config-test-mode)=
## `test-mode`

Whether the model is intended for testing.
If true, accessing the charm store does not affect statistical
data of the store. (default false).

**Default value:** `false`

**Type:** bool


(model-config-transmit-vendor-metrics)=
## `transmit-vendor-metrics`

Determines whether metrics declared by charms deployed into this model are sent for anonymized aggregate analytics.

**Default value:** `true`

**Type:** bool


(model-config-type)=
## `type`

*Note: This value cannot be changed after model creation.* 

*Note: This value must be set.* 

Type of model, e.g. local, ec2.

**Default value:** `""`

**Type:** string


(model-config-update-status-hook-interval)=
## `update-status-hook-interval`

How often to run the charm update-status hook, in human-readable time format (default 5m, range 1-60m).

**Default value:** `5m`

**Type:** string


(model-config-uuid)=
## `uuid`

*Note: This value is set by Juju.*

The UUID of the model.

**Default value:** `""`

**Type:** string


