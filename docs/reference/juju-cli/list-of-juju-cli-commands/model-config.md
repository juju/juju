(command-juju-model-config)=
# `juju model-config`

```
Usage: juju model-config [options] [<model-key>[=<value>] ...]

Summary:
Displays or sets configuration values on a model.

Global Options:
--debug  (= false)
    equivalent to --show-log --logging-config=<root>=DEBUG
-h, --help  (= false)
    Show help on a command or other topic.
--logging-config (= "")
    specify log levels for modules
--quiet  (= false)
    show no informational output
--show-log  (= false)
    if set, write the log file to stderr
--verbose  (= false)
    show more verbose output

Command Options:
-B, --no-browser-login  (= false)
    Do not use web browser for authentication
--format  (= tabular)
    Specify output format (json|tabular|yaml)
--ignore-agent-version  (= false)
    Skip the error when passing in the agent version configuration (deprecated)
--ignore-read-only-fields  (= false)
    Ignore read only fields that might cause errors to be emitted while processing yaml documents
-m, --model (= "")
    Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>
-o, --output (= "")
    Specify an output file
--reset  (= )
    Reset the provided comma delimited keys, deletes keys not in the model config

Details:
By default, all configuration (keys, source, and values) for the current model
are displayed.

Supplying one key name returns only the value for the key. Supplying key=value
will set the supplied key to the supplied value, this can be repeated for
multiple keys. You can also specify a yaml file containing key values, that can
be used for the input for the command.

Model config yaml can be piped from stdin from the output of the command stdout.
Some model-config configuration are read-only, to prevent the command exiting on
read-only fields, setting "ignore-read-only-fields" will cause it to skip over
the fields when they're encountered.

The reset flag will set the provided key(s) to the model default for those key(s).
Any key not in the default model config, will be deleted.

The following keys are available:

agent-metadata-url:
  type: string
  description: URL of private stream
agent-stream:
  type: string
  description: Version of Juju to use for deploy/upgrades.
apt-ftp-proxy:
  type: string
  description: The APT FTP proxy for the model
apt-http-proxy:
  type: string
  description: The APT HTTP proxy for the model
apt-https-proxy:
  type: string
  description: The APT HTTPS proxy for the model
apt-mirror:
  type: string
  description: The APT mirror for the model
apt-no-proxy:
  type: string
  description: List of domain addresses not to be proxied for APT (comma-separated)
authorized-keys:
  type: string
  description: Any authorized SSH public keys for the model, as found in a ~/.ssh/authorized_keys
    file
automatically-retry-hooks:
  type: bool
  description: Determines whether the uniter should automatically retry failed hooks
backup-dir:
  type: string
  description: Directory used to store the backup working directory
charmhub-url:
  type: string
  description: The url for CharmHub API calls
cloudinit-userdata:
  type: string
  description: Cloud-init user-data (in yaml format) to be added to userdata for new
    machines created in this model
container-image-metadata-url:
  type: string
  description: The URL at which the metadata used to locate container OS image ids
    is located
container-image-stream:
  type: string
  description: The simplestreams stream used to identify which image ids to search
    when starting a container.
container-inherit-properties:
  type: string
  description: List of properties to be copied from the host machine to new containers
    created in this model (comma-separated)
container-networking-method:
  type: string
  description: Method of container networking setup - one of fan, provider, local
default-series:
  type: string
  description: The default series of Ubuntu to use for deploying charms, will act
    like --series when deploying charms
default-space:
  type: string
  description: The default network space used for application endpoints in this model
development:
  type: bool
  description: Whether the model is in development mode
disable-network-management:
  type: bool
  description: Whether the provider should control networks (on MAAS models, set to
    true for MAAS to control networks
disable-telemetry:
  type: bool
  description: Disable telemetry reporting of model information
egress-subnets:
  type: string
  description: Source address(es) for traffic originating from this model
enable-os-refresh-update:
  type: bool
  description: Whether newly provisioned instances should run their respective OS's
    update capability.
enable-os-upgrade:
  type: bool
  description: Whether newly provisioned instances should run their respective OS's
    upgrade capability.
extra-info:
  type: string
  description: Arbitrary user specified string data that is stored against the model.
fan-config:
  type: string
  description: Configuration for fan networking for this model
firewall-mode:
  type: string
  description: |-
    The mode to use for network firewalling.

    'instance' requests the use of an individual firewall per instance.

    'global' uses a single firewall for all instances (access
    for a network port is enabled to one instance if any instance requires
    that port).

    'none' requests that no firewalling should be performed
    inside the model. It's useful for clouds without support for either
    global or per instance security groups.
ftp-proxy:
  type: string
  description: The FTP proxy value to configure on instances, in the FTP_PROXY environment
    variable
gui-stream:
  type: string
  description: The simplestreams stream used to identify which gui ids to search when
    downloading a gui tarball.
http-proxy:
  type: string
  description: The HTTP proxy value to configure on instances, in the HTTP_PROXY environment
    variable
https-proxy:
  type: string
  description: The HTTPS proxy value to configure on instances, in the HTTPS_PROXY
    environment variable
ignore-machine-addresses:
  type: bool
  description: Whether the machine worker should discover machine addresses on startup
image-metadata-url:
  type: string
  description: The URL at which the metadata used to locate OS image ids is located
image-stream:
  type: string
  description: The simplestreams stream used to identify which image ids to search
    when starting an instance.
juju-ftp-proxy:
  type: string
  description: The FTP proxy value to pass to charms in the JUJU_CHARM_FTP_PROXY environment
    variable
juju-http-proxy:
  type: string
  description: The HTTP proxy value to pass to charms in the JUJU_CHARM_HTTP_PROXY
    environment variable
juju-https-proxy:
  type: string
  description: The HTTPS proxy value to pass to charms in the JUJU_CHARM_HTTPS_PROXY
    environment variable
juju-no-proxy:
  type: string
  description: List of domain addresses not to be proxied (comma-separated), may contain
    CIDRs. Passed to charms in the JUJU_CHARM_NO_PROXY environment variable
logforward-enabled:
  type: bool
  description: Whether syslog forwarding is enabled.
logging-config:
  type: string
  description: The configuration string to use when configuring Juju agent logging
    (see http://godoc.org/github.com/juju/loggo#ParseConfigurationString for details)
logging-output:
  type: string
  description: 'The logging output destination: database and/or syslog. (default "")'
lxd-snap-channel:
  type: string
  description: The channel to use when installing LXD from a snap (cosmic and later)
max-action-results-age:
  type: string
  description: The maximum age for action entries before they are pruned, in human-readable
    time format
max-action-results-size:
  type: string
  description: The maximum size for the action collection, in human-readable memory
    format
max-status-history-age:
  type: string
  description: The maximum age for status history entries before they are pruned,
    in human-readable time format
max-status-history-size:
  type: string
  description: The maximum size for the status history collection, in human-readable
    memory format
mode:
  type: string
  description: |-
    Mode sets the type of mode the model should run in.
    If the mode is set to "strict" then errors will be used instead of
    using fallbacks. By default mode is set to be lenient and use fallbacks
    where possible. (default "")
net-bond-reconfigure-delay:
  type: int
  description: The amount of time in seconds to sleep between ifdown and ifup when
    bridging
no-proxy:
  type: string
  description: List of domain addresses not to be proxied (comma-separated)
num-container-provision-workers:
  type: int
  description: The number of container provisioning workers to use per machine
num-provision-workers:
  type: int
  description: The number of provisioning workers to use per model
provisioner-harvest-mode:
  type: string
  description: What to do with unknown machines (default destroyed)
proxy-ssh:
  type: bool
  description: Whether SSH commands should be proxied through the API server
resource-tags:
  type: attrs
  description: resource tags
snap-http-proxy:
  type: string
  description: The HTTP proxy value to for installing snaps
snap-https-proxy:
  type: string
  description: The HTTPS proxy value to for installing snaps
snap-store-assertions:
  type: string
  description: The assertions for the defined snap store proxy
snap-store-proxy:
  type: string
  description: The snap store proxy for installing snaps
snap-store-proxy-url:
  type: string
  description: The URL for the defined snap store proxy
ssl-hostname-verification:
  type: bool
  description: Whether SSL hostname verification is enabled (default true)
storage-default-block-source:
  type: string
  description: The default block storage source for the model
storage-default-filesystem-source:
  type: string
  description: The default filesystem storage source for the model
syslog-ca-cert:
  type: string
  description: The certificate of the CA that signed the syslog server certificate,
    in PEM format.
syslog-client-cert:
  type: string
  description: The syslog client certificate in PEM format.
syslog-client-key:
  type: string
  description: The syslog client key in PEM format.
syslog-host:
  type: string
  description: The hostname:port of the syslog server.
test-mode:
  type: bool
  description: |-
    Whether the model is intended for testing.
    If true, accessing the charm store does not affect statistical
    data of the store. (default false)
transmit-vendor-metrics:
  type: bool
  description: Determines whether metrics declared by charms deployed into this model
    are sent for anonymized aggregate analytics
update-status-hook-interval:
  type: string
  description: How often to run the charm update-status hook, in human-readable time
    format (default 5m, range 1-60m)

Examples:

Print the value of default-series:
    juju model-config default-series

Print the model config of model mycontroller:mymodel:
    juju model-config -m mycontroller:mymodel

Set the value of ftp-proxy to 10.0.0.1:8000:
    juju model-config ftp-proxy=10.0.0.1:8000

Set the value of ftp-proxy to 10.0.0.1:8000, and set the values defined in
path/to/file.yaml:
    juju model-config ftp-proxy=10.0.0.1:8000 path/to/file.yaml

Set the model config key value pairs defined in a file:
    juju model-config path/to/file.yaml

Set model config values of a specific model:
    juju model-config -m othercontroller:mymodel default-series=yakkety test-mode=false

Reset the values of the provided keys to model defaults:
    juju model-config --reset default-series,test-mode

See also:
    models
    model-defaults
    show-cloud
    controller-config
```