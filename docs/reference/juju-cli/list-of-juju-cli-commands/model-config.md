(command-juju-model-config)=
# `juju model-config`
> See also: [models](#models), [model-defaults](#model-defaults), [show-cloud](#show-cloud), [controller-config](#controller-config)

## Summary
Displays or sets configuration values on a model.

## Usage
```juju model-config [options] [<model-key>[=<value>] ...]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--color` | false | Use ANSI color codes in output |
| `--file` |  | path to yaml-formatted configuration file |
| `--format` | tabular | Specify output format (json&#x7c;tabular&#x7c;yaml) |
| `--ignore-read-only-fields` | false | Ignore read only fields that might cause errors to be emitted while processing yaml documents |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `--no-color` | false | Disable ANSI color codes in tabular output |
| `-o`, `--output` |  | Specify an output file |
| `--reset` |  | Reset the provided comma delimited keys |

## Examples

Print the value of default-base:

    juju model-config default-base

Print the model config of model mycontroller:mymodel:

    juju model-config -m mycontroller:mymodel

Set the value of ftp-proxy to 10.0.0.1:8000:

    juju model-config ftp-proxy=10.0.0.1:8000

Set the model config to key=value pairs defined in a file:

    juju model-config --file path/to/file.yaml

Set model config values of a specific model:

    juju model-config -m othercontroller:mymodel default-base=ubuntu@22.04 test-mode=false

Reset the values of the provided keys to model defaults:

    juju model-config --reset default-base,test-mode


## Details

To view all configuration values for the current model, run
    juju model-config
You can target a specific model using the -m flag:
    juju model-config -m <model>
    juju model-config -m <controller>:<model>
By default, the config will be printed in a tabular format. You can instead
print it in json or yaml format using the --format flag:
    juju model-config --format json
    juju model-config --format yaml

To view the value of a single config key, run
    juju model-config key
To set config values, run
    juju model-config key1=val1 key2=val2 ...
You can also reset config keys to their default values:
    juju model-config --reset key1
    juju model-config --reset key1,key2,key3
You may simultaneously set some keys and reset others:
    juju model-config key1=val1 key2=val2 --reset key3,key4

Config values can be imported from a yaml file using the --file flag:
    juju model-config --file=path/to/cfg.yaml
This allows you to e.g. save a model's config to a file:
    juju model-config --format=yaml > cfg.yaml
and then import the config later. Note that the output of model-config
may include read-only values, which will cause an error when importing later.
To prevent the error, use the --ignore-read-only-fields flag:
    juju model-config --file=cfg.yaml --ignore-read-only-fields

You can also read from stdin using "-", which allows you to pipe config values
from one model to another:
    juju model-config -c c1 --format=yaml \
      | juju model-config -c c2 --file=- --ignore-read-only-fields
You can simultaneously read config from a yaml file and set config keys
as above. The command-line args will override any values specified in the file.

The default-series key is deprecated in favour of default-base
e.g. default-base=ubuntu@22.04.

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
    container-image-metadata-defaults-disabled:
      type: bool
      description: Whether default simplestreams sources are used for image metadata with
        containers.
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
      description: Method of container networking setup - one of "provider", "local",
        or "" (auto-configure).
    default-base:
      type: string
      description: The default base image to use for deploying charms, will act like --base
        when deploying charms
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
    firewall-mode:
      type: string
      description: The mode to use for network firewalling.
    ftp-proxy:
      type: string
      description: The FTP proxy value to configure on instances, in the `FTP_PROXY` environment
        variable
    http-proxy:
      type: string
      description: The HTTP proxy value to configure on instances, in the `HTTP_PROXY`
        environment variable
    https-proxy:
      type: string
      description: The HTTPS proxy value to configure on instances, in the `HTTPS_PROXY`
        environment variable
    ignore-machine-addresses:
      type: bool
      description: Whether the machine worker should discover machine addresses on startup
    image-metadata-defaults-disabled:
      type: bool
      description: Whether default simplestreams sources are used for image metadata.
    image-metadata-url:
      type: string
      description: The URL at which the metadata used to locate OS image ids is located
    image-stream:
      type: string
      description: The simplestreams stream used to identify which image ids to search
        when starting an instance.
    juju-ftp-proxy:
      type: string
      description: The FTP proxy value to pass to charms in the `JUJU_CHARM_FTP_PROXY`
        environment variable
    juju-http-proxy:
      type: string
      description: The HTTP proxy value to pass to charms in the `JUJU_CHARM_HTTP_PROXY`
        environment variable
    juju-https-proxy:
      type: string
      description: The HTTPS proxy value to pass to charms in the `JUJU_CHARM_HTTPS_PROXY`
        environment variable
    juju-no-proxy:
      type: string
      description: List of domain addresses not to be proxied (comma-separated), may contain
        CIDRs. Passed to charms in the `JUJU_CHARM_NO_PROXY` environment variable
    logging-config:
      type: string
      description: The configuration string to use when configuring Juju agent logging
        (see [this link](https://pkg.go.dev/github.com/juju/loggo#ParseConfigString) for
        details)
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
    mode:
      type: string
      description: |-
        Mode is a comma-separated list which sets the
        mode the model should run in. So far only one is implemented
        - If 'requires-prompts' is present, clients will ask for confirmation before removing
        potentially valuable resources.
        (default "")
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
    saas-ingress-allow:
      type: string
      description: |-
        Application-offer ingress allowlist is a comma-separated list of
        CIDRs specifying what ingress can be applied to offers in this model.
    snap-http-proxy:
      type: string
      description: The HTTP proxy value for installing snaps
    snap-https-proxy:
      type: string
      description: The HTTPS proxy value for installing snaps
    snap-store-assertions:
      type: string
      description: The assertions for the defined snap store proxy
    snap-store-proxy:
      type: string
      description: The snap store proxy for installing snaps
    snap-store-proxy-url:
      type: string
      description: The URL for the defined snap store proxy
    ssh-allow:
      type: string
      description: |-
        SSH allowlist is a comma-separated list of CIDRs from
        which machines in this model will accept connections to the SSH service.
        Currently only the aws & openstack providers support ssh-allow
    ssl-hostname-verification:
      type: bool
      description: Whether SSL hostname verification is enabled (default true)
    storage-default-block-source:
      type: string
      description: The default block storage source for the model
    storage-default-filesystem-source:
      type: string
      description: The default filesystem storage source for the model
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