(list-of-model-configuration-keys)=
# List of model configuration keys


This document gives a list of all the configuration keys that can be applied to a Juju model.
## agent-metadata-url

URL of private stream.

**Type:** string


## agent-stream

Version of Juju to use for deploy/upgrades.

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

	juju bootstrap aws --auto-upgrade.

**Type:** string

**Default value:** released


## agent-version

*Note: This value is set by Juju.*

*Note: This value cannot be changed after model creation.* 

The desired Juju agent version to use.

**Type:** string


## apt-ftp-proxy

The APT FTP proxy for the model.

**Type:** string


## apt-http-proxy

The APT HTTP proxy for the model.

**Type:** string


## apt-https-proxy

The APT HTTPS proxy for the model.

**Type:** string


## apt-mirror

The APT mirror for the model

The APT packaging system is used to install and upgrade software on machines
provisioned in the model, and many charms also use APT to install software for
the applications they deploy. It is possible to set a specific mirror for the
APT packages to use, by setting ‘apt-mirror’:

	juju model-config apt-mirror=http://archive.ubuntu.com/ubuntu/

To restore the default behaviour you would run:

	juju model-config --reset apt-mirror

The apt-mirror option is often used to point to a local mirror.

**Type:** string


## apt-no-proxy

List of domain addresses not to be proxied for APT (comma-separated).

**Type:** string


## automatically-retry-hooks

Determines whether the uniter should automatically retry failed hooks

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
	juju resolved unit-name/# .

**Type:** bool

**Default value:** true


## backup-dir

Directory used to store the backup working directory.

**Type:** string


## charmhub-url

The url for CharmHub API calls.

**Type:** string

**Default value:** https://api.charmhub.io


## cloudinit-userdata

Cloud-init user-data (in yaml format) to be added to userdata for new machines created in this model.

**Type:** string


## container-image-metadata-defaults-disabled

Whether default simplestreams sources are used for image metadata with containers.

**Type:** bool

**Default value:** false


## container-image-metadata-url

The URL at which the metadata used to locate container OS image ids is located.

**Type:** string


## container-image-stream

The simplestreams stream used to identify which image ids to search when starting a container.

**Type:** string

**Default value:** released


## container-inherit-properties

List of properties to be copied from the host machine to new containers created in this model (comma-separated).

**Type:** string


## container-networking-method

Method of container networking setup - one of "provider", "local", or "" (auto-configure).

**Type:** string


## default-base

The default base image to use for deploying charms, will act like --base when deploying charms.

**Type:** string


## default-space

The default network space used for application endpoints in this model.

**Type:** string


## development

Whether the model is in development mode.

**Type:** bool

**Default value:** false


## disable-network-management

Whether the provider should control networks (on MAAS models, set to true for MAAS to control networks.

**Type:** bool

**Default value:** false


## disable-telemetry

Disable telemetry reporting of model information.

**Type:** bool

**Default value:** false


## egress-subnets

Source address(es) for traffic originating from this model.

**Type:** string


## enable-os-refresh-update

Whether newly provisioned instances should run their respective OS's update capability.

**Type:** bool

**Default value:** true


## enable-os-upgrade

Whether newly provisioned instances should run their respective OS's upgrade capability.

**Type:** bool

**Default value:** true


## extra-info

Arbitrary user specified string data that is stored against the model.

**Type:** string


## firewall-mode

*Note: This value cannot be changed after model creation.* 

The mode to use for network firewalling.

'instance' requests the use of an individual firewall per instance.

'global' uses a single firewall for all instances (access
for a network port is enabled to one instance if any instance requires
that port).

'none' requests that no firewalling should be performed
inside the model. It's useful for clouds without support for either
global or per instance security groups.

**Type:** string

**Default value:** instance

**Valid values:** instance, global, none


## ftp-proxy

The FTP proxy value to configure on instances, in the FTP_PROXY environment variable.

**Type:** string


## http-proxy

The HTTP proxy value to configure on instances, in the HTTP_PROXY environment variable.

**Type:** string


## https-proxy

The HTTPS proxy value to configure on instances, in the HTTPS_PROXY environment variable.

**Type:** string


## ignore-machine-addresses

Whether the machine worker should discover machine addresses on startup.

**Type:** bool

**Default value:** false


## image-metadata-defaults-disabled

Whether default simplestreams sources are used for image metadata.

**Type:** bool

**Default value:** false


## image-metadata-url

The URL at which the metadata used to locate OS image ids is located.

**Type:** string


## image-stream

The simplestreams stream used to identify which image ids to search when starting an instance.

**Type:** string

**Default value:** released


## juju-ftp-proxy

The FTP proxy value to pass to charms in the JUJU_CHARM_FTP_PROXY environment variable.

**Type:** string


## juju-http-proxy

The HTTP proxy value to pass to charms in the JUJU_CHARM_HTTP_PROXY environment variable.

**Type:** string


## juju-https-proxy

The HTTPS proxy value to pass to charms in the JUJU_CHARM_HTTPS_PROXY environment variable.

**Type:** string


## juju-no-proxy

List of domain addresses not to be proxied (comma-separated), may contain CIDRs. Passed to charms in the JUJU_CHARM_NO_PROXY environment variable.

**Type:** string

**Default value:** 127.0.0.1,localhost,::1


## logging-config

The configuration string to use when configuring Juju agent logging (see http://godoc.org/github.com/juju/loggo#ParseConfigurationString for details).

**Type:** string


## lxd-snap-channel

The channel to use when installing LXD from a snap (cosmic and later).

**Type:** string

**Default value:** 5.0/stable


## max-action-results-age

The maximum age for action entries before they are pruned, in human-readable time format.

**Type:** string

**Default value:** 336h


## max-action-results-size

The maximum size for the action collection, in human-readable memory format.

**Type:** string

**Default value:** 5G


## max-status-history-age

The maximum age for status history entries before they are pruned, in human-readable time format.

**Type:** string

**Default value:** 336h

## max-status-history-size

The maximum size for the status history collection, in human-readable memory format.

**Type:** string

**Default value:** 5G


## mode

Mode is a comma-separated list which sets the
mode the model should run in. So far only one is implemented
- If 'requires-prompts' is present, clients will ask for confirmation before removing
potentially valuable resources.
(default "").

**Type:** string

**Default value:** requires-prompts


## name

*Note: This value cannot be changed after model creation.* 

*Note: This value must be set.* 

The name of the current model.

**Type:** string


## net-bond-reconfigure-delay

The amount of time in seconds to sleep between ifdown and ifup when bridging.

**Type:** int

**Default value:** 17


## no-proxy

List of domain addresses not to be proxied (comma-separated).

**Type:** string

**Default value:** 127.0.0.1,localhost,::1


## num-container-provision-workers

The number of container provisioning workers to use per machine.

**Type:** int

**Default value:** 4


## num-provision-workers

The number of provisioning workers to use per model.

**Type:** int

**Default value:** 16


## provisioner-harvest-mode

What to do with unknown machines (default destroyed).

**Type:** string

**Default value:** destroyed

**Valid values:** all, none, unknown, destroyed


## proxy-ssh

Whether SSH commands should be proxied through the API server.

**Type:** bool

**Default value:** false


## resource-tags

resource tags.

**Type:** attrs


## saas-ingress-allow

Application-offer ingress allowlist is a comma-separated list of
CIDRs specifying what ingress can be applied to offers in this model.

**Type:** string

**Default value:** 0.0.0.0/0,::/0


## snap-http-proxy

The HTTP proxy value for installing snaps.

**Type:** string


## snap-https-proxy

The HTTPS proxy value for installing snaps.

**Type:** string


## snap-store-assertions

The assertions for the defined snap store proxy.

**Type:** string


## snap-store-proxy

The snap store proxy for installing snaps.

**Type:** string


## snap-store-proxy-url

The URL for the defined snap store proxy.

**Type:** string


## ssh-allow

SSH allowlist is a comma-separated list of CIDRs from
which machines in this model will accept connections to the SSH service.
Currently only the aws & openstack providers support ssh-allow.

**Type:** string

**Default value:** 0.0.0.0/0,::/0


## ssl-hostname-verification

Whether SSL hostname verification is enabled (default true).

**Type:** bool

**Default value:** true


## storage-default-block-source

The default block storage source for the model.

**Type:** string


## storage-default-filesystem-source

The default filesystem storage source for the model.

**Type:** string


## test-mode

Whether the model is intended for testing.
If true, accessing the charm store does not affect statistical
data of the store. (default false).

**Type:** bool

**Default value:** false


## transmit-vendor-metrics

Determines whether metrics declared by charms deployed into this model are sent for anonymized aggregate analytics.

**Type:** bool

**Default value:** true


## type

*Note: This value cannot be changed after model creation.* 

*Note: This value must be set.* 

Type of model, e.g. local, ec2.

**Type:** string


## update-status-hook-interval

How often to run the charm update-status hook, in human-readable time format (default 5m, range 1-60m).

**Type:** string

**Default value:** 5m


## uuid

*Note: This value is set by Juju.*

*Note: This value cannot be changed after model creation.* 

The UUID of the model.

**Type:** string


