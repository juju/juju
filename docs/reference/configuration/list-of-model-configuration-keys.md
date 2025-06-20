(list-of-model-configuration-keys)=
# List of model configuration keys


This document gives a list of all the configuration keys that can be applied to a Juju model.

```{important}

Some are only defined for a given cloud; see {ref}`model-config-cloud-specific-key`. Others are defined generally but may still only be available for some clouds; e.g., {ref}`model-config-container-inherit-properties`.

```

(model-config-cloud-specific-key)=
## `<cloud-specific key>`

> See {ref}`list-of-supported-clouds`> `<cloud name>` > Cloud > definition <list-of-supported-clouds>` or run `juju show-cloud <cloud> --include-config`.


(model-config-agent-metadata-url)=
## `agent-metadata-url`

`agent-metadata-url` is the URL of the private stream.

**Type:** string

**Default value:** ""


## `agent-stream`

`agent-stream` is the version of Juju to use for deploy/upgrades.

**Type:** string

**Default value:** ""

**Valid values:** `released`, `devel`, `proposed`

## `agent-version`

`agent-version` is the desired Juju agent version to use.

> See more: {ref}`agent`

**Type:** string

**Details:**

The `agent-stream` key specifies the "stream" to use when a Juju agent is to be installed or upgraded. This setting reflects the general stability of the software and defaults to 'released', indicating that only the latest stable version is to be used.

To run the upcoming stable release (before it has passed the normal QA process) you can set:

``` yaml
agent-stream: proposed
```

For testing purposes, you can use the latest unstable version by setting:

``` yaml
agent-stream: devel
```

The `agent-version` option specifies a "patch version" for the agent that is to be installed on a new controller relative to the Juju client's current major.minor version (Juju uses a major.minor.patch numbering scheme).

For example, Juju 2.3.2 means major version 2, minor version 3, and patch version 2. On a client system with this release of Juju installed, the machine agent's version for a newly-created controller would be the same. To specify a patch version of 1 (instead of 2), the following would be run:

``` text
juju bootstrap aws --agent-version='2.3.1'
```

If a patch version is available that is greater than that of the client then it can be targeted in this way:

``` text
juju bootstrap aws --auto-upgrade
```

(model-config-apt-ftp-proxy)=
## `apt-ftp-proxy`

`apt-ftp-proxy` is the APT FTP proxy for the model.

**Type:** string

**Default value:** ""

(model-config-apt-http-proxy)=
## `apt-http-proxy`

`apt-http-proxy` is the APT HTTP proxy for the model.

**Type:** string

**Default value:** ""

(model-config-apt-https-proxy)=
## `apt-https-proxy`

`apt-https-proxy` is the APT HTTPS proxy for the model.

**Type:** string

**Default value:** ""

(model-config-apt-mirror)=
## `apt-mirror`

`apt-mirror` is the APT mirror for the model.

**Type:** string

**Default value:** ""

**Details:**

The APT packaging system is used to install and upgrade software on machines provisioned in the model, and many charms also use APT to install software for the applications they deploy. It is possible to set a specific mirror for the APT packages to use, by setting 'apt-mirror':

``` text
juju model-config apt-mirror=http://archive.ubuntu.com/ubuntu/
```

To restore the default behaviour you would run:

``` text
juju model-config --reset apt-mirror
```

The `apt-mirror` option is often used to point to a local mirror.

(model-config-apt-no-proxy)=
## `apt-no-proxy`


`apt-no-proxy` is the list of domain addresses not to be proxied for APT (comma-separated).

**Type:** string

**Default value:** ""

## `automatically-retry-hooks`

`automatically-retry-hooks` determines whether the uniter should automatically retry failed hooks.

**Type:** boolean

**Default value:** true

**Details:**

Juju retries failed hooks automatically using an exponential backoff algorithm. They will be retried after 5, 10, 20, 40 seconds up to a period of 5 minutes, and then every 5 minutes. The logic behind this is that some hook errors are caused by timing issues or the temporary unavailability of other applications - automatic retry enables the Juju model to heal itself without troubling the user.

However, in some circumstances, such as debugging charms, this behaviour can be distracting and unwelcome. For this reason, it is possible to set the `automatically-retry-hooks` option to 'false' to disable this behaviour. In this case, users will have to manually retry any hook which fails, using the command above, as with earlier versions of Juju.

```{important}

Even with the automatic retry enabled, it is still possible to use the `juju resolved unit-name/#` command to retry manually.

```

## `backup-dir`

`backup-dir` is the directory used to store the backup working directory.

**Type:** string

**Default value:** ""


## `charmhub-url`

`charmhub-url`  is the url for Charmhub API calls.

**Type:**

**Default value:** [https://api.charmhub.io](https://api.charmhub.io)

## `cloudinit-userdata`

```{caution}

This is a sharp knife feature - be careful with it.

```

`cloudinit-userdata` is the cloud-init user-data (in yaml format) to be added to userdata for new machines created in this model.

**Type:** string

**Default value:** ""

**Details:**

<!--It was added to Juju in version 2.3.1.-->


The `cloudinit-userdata` allows the user to provide additional cloudinit data to be included in the cloudinit data created by Juju.

Specifying a key will overwrite what juju puts in the cloudinit file with the following caveats:
1. `users` and `bootcmd` keys will cause an error
2. The `packages` key will be appended to the packages listed by juju
3. The `runcmds` key will cause an error.  You can specify `preruncmd` and `postruncmd` keys to prepend and append the runcmd created by Juju.


### Use cases

- setting a default locale for deployments that wish to use their own locale settings
- adding custom CA certificates for models that are sitting behind an HTTPS proxy
- adding a private apt mirror to enable private packages to be installed
- add SSH fingerprints to a deny list to prevent them from being printed to the console for security-focused deployments

### Background

Juju uses [`cloud-init`](https://cloud-init.io/) to customise instances once they have been provisioned by the cloud. The `cloudinit-userdata` model configuration setting (model config) allows you to tweak what happens to machines when they are created up via the "user data" feature.

From the website:

> Cloud images are operating system templates and every instance starts out as an identical clone of every other instance. *It is the user data that gives every cloud instance its personality and cloud-init is the tool that applies user data to your instances automatically.*

### How-to

#### Provide custom user data to cloudinit

Create a file, `cloudinit-userdata.yaml`, which starts with the `cloudinit-userdata` key and data you wish to include in the cloudinit file.  Note: juju reads the value as a string, though formatted as YAML.

Template `cloudinit-userdata.yaml`:

```text
cloudinit-userdata: |
    <key>: <value>
    <key>: <value>
```

Provide the path your file to the `model-config` command:


```text
juju model-config --file cloudinit-userdata.yaml
```

#### Read the current setting

To read the current value, provide the `cloudinit-userdata` key to the `model-config` command as a command-line parameter. Adding the `--format yaml` option ensures that it is properly formatted.

```text
juju model-config cloudinit-userdata --format yaml
```

Sample output:

    cloudinit-userdata: |
      packages:
        - 'python-keystoneclient'
        - 'python-glanceclient'

#### Clear the current custom user data

Use the `--reset` option to the `model-config` command to clear anything that has been previously set.

```text
juju model-config --reset cloudinit-userdata
```

### Known issues

- custom cloudinit-userdata must be passed via file, not as options on the command line (like the `config` command)

(model-config-container-image-metadata-url)=
## `container-image-metadata-url`

`container-image-metadata-url` is the URL at which the metadata used to locate container OS image ids is located.

**Type:** string

**Default value:** "

**Valid values:** url

## `container-image-stream`

`container-image-stream`  is the simplestreams stream used to identify which image ids to search when starting a container.

**Type:** string

**Default value:** `released`

**Valid values:** url

(model-config-container-inherit-properties)=
## `container-inherit-properties`

`container-inherit-properties` is the list of properties to be copied from the host machine to new containers created in this model (comma-separated).

**Type:** string

**Default value:** ""

**Details:**

The `container-inherit-properties` key allows for a limited set of parameters enabled on a Juju machine to be inherited by any hosted containers (KVM guests or LXD containers). The machine and container must be running the same series.

```{important}

This key is only supported by the MAAS provider.

```

The parameters are:

- apt-primary
- apt-security
- apt-sources
- ca-certs

For MAAS `v.2.5` or greater the parameters are:

- apt-sources
- ca-certs

For example:

```text
juju model-config container-inherit-properties="ca-certs, apt-sources"
```

<!--Old content of this doc. It seems to have been incorporated into the one above, copied from the list of model configs.


This key allows the user to specify cloudinit keys to be copied from host machines to containers on the host from the vendor files.

Included in juju 2.4-beta1 as of early Feb.

Using:
--
Caveats related to series: If using a Trusty machine, only Trusty containers will use this feature.  OS type must be the same between machine and container.

Allowed keys are: ca-certs, apt-primary, apt-security, apt-sources.  In xenial and other series (not trusty):
* apt-primary finds:
    apt:
      primary:
        …
* apt-security finds:
    apt:
      security:
        …
* apt-sources finds:
    apt:
      sources:
        …

In trusty apt-security is ignored (unless someone can provide a map):

* apt-primary finds:
    apt_mirror: ...
    apt_mirror_search: ...
    apt_mirror_search_dns: ...
* apt-sources finds:
    apt_sources: ...


`juju model-config container-inherit-properties=”ca-certs, apt-primary”`
-->

## `container-networking-method`

`container-networking-method` is the method of container networking setup - one of fan, provider, local.

**Type:** string

**Valid values:** `local`, `provider`, `fan`

## `default-base`

`default-base` is the default base image to use for deploying charms, will act like `--base` when deploying charms.

**Type:** string

**Default value:** ""

## `default-space`

`default-space` is the default network space used for application endpoints in this model.

**Type:** string

**Default value:** ""

## `development`

`development` determines whether the model is in development mode.

**Type:** boolean

**Default value:** false

## `disable-network-management`

`disable-network-management` determines whether the provider should control networks (on MAAS models, set to true for MAAS to control networks).

**Type:** boolean

**Default value:** false

**Details:**

This key can only be used with MAAS models and should otherwise be set to 'false' (default) unless you want to take over network control from Juju because you have unique and well-defined needs. Setting this to 'true' with MAAS gives you the same behaviour with containers as you already have with other providers: one machine-local address on a single network interface, bridged to the default bridge.

## `disable-telemetry`

`disable-telemetry` disables telemetry reporting of model information.

**Type:** boolean

**Default value:** false

## `egress-subnets`

`egress-subnets` is the source address(es) for traffic originating from this model.

**Type:** string

**Default value:** ""

## `enable-os-refresh-update`

`enable-os-refresh-update` determines whether newly provisioned instances should run their respective OS's update capability.

**Type:** boolean

**Default value:** true

**Details:**

When Juju provisions a machine, its default behaviour is to upgrade existing packages to their latest version. If your OS images are fresh and/or your deployed applications do not require the latest package versions, you can disable upgrades in order to provision machines faster.

Two boolean configuration options are available to disable APT updates and upgrades: `enable-os-refresh-update` (apt update) and `enable-os-upgrade` (apt upgrade), respectively.

```yaml
enable-os-refresh-update: false
enable-os-upgrade: false
```

You may also want to just update the package list to ensure a charm has the latest software available to it by disabling upgrades but enabling updates.

## `enable-os-upgrade`

`enable-os-upgrade` determines whether newly provisioned instances should run their respective OS's upgrade capability.

**Type:** boolean

**Default value:** true

**Details:**

When Juju provisions a machine, its default behaviour is to upgrade existing packages to their latest version. If your OS images are fresh and/or your deployed applications do not require the latest package versions, you can disable upgrades in order to provision machines faster.

Two Boolean configuration options are available to disable APT updates and upgrades: `enable-os-refresh-update` (apt update) and `enable-os-upgrade` (apt upgrade), respectively.

``` yaml
enable-os-refresh-update: false
enable-os-upgrade: false
```

You may also want to just update the package list to ensure a charm has the latest software available to it by disabling upgrades but enabling updates.

## `fan-config`

`fan-config`  is the configuration for fan networking for this model.

**Type:** string

**Default value:** ""

**Valid values:** `overlay_CIDR<par>=<par>underlay_CIDR`


## `firewall-mode`

`firewall-mode` is the mode to use for network firewalling. It's useful for clouds without support for either global or per instance security groups.

**Type:** string

**Default value:** `instance`

**Valid values:** `instance`, `global`, `none`. `instance` requests the use of an individual firewall per instance; `global` uses a single firewall for all instances (access for a network port is enabled to one instance if any instance requires that port); `none` requests that no firewalling should be performed inside the model.

(model-config-ftp-proxy)=
## `ftp-proxy`

`ftp-proxy` is the FTP proxy value to configure on instances, in the `FTP_PROXY` environment variable.

**Type:** string

**Default value:** ""

**Valid values:** url

(model-config-http-proxy)=
## `http-proxy`

`http-proxy`  is the HTTP proxy value to configure on instances, in the `HTTP_PROXY` environment variable.

**Type:** string

**Default value:** ""

**Valid values:** url

(model-config-https-proxy)=
## `https-proxy`

`https-proxy`  is the HTTPS proxy value to configure on instances, in the `HTTPS_PROXY` environment variable.

**Type:** string

**Default value:** ""

**Valid values:** url


## `ignore-machine-addresses`

`ignore-machine-addresses` determines whether the machine worker should discover machine addresses on startup.

**Type:** boolean

**Default value:** false

**Valid values:**

(model-config-image-metadata-url)=
## `image-metadata-url`

`image-metadata-url` is the URL at which the metadata used to locate OS image ids is located.

**Type:** string

**Default value:** ""

**Valid values:** url


## `image-stream`

`image-stream` is the simplestreams stream used to identify which image ids to search when starting an instance.

**Type:** string

**Default value:** `released

**Details:**

Juju, by default, uses the slow-changing 'released' images when provisioning machines. However, the `image-stream` option can be set to 'daily' to use more up-to-date images, thus shortening the time it takes to perform APT package upgrades.

(model-config-juju-ftp-proxy)=
## `juju-ftp-proxy`

`juju-ftp-proxy` is the FTP proxy value to pass to charms in the `JUJU_CHARM_FTP_PROXY` environment variable.

**Type:** string

**Default value:** ""

(model-config-juju-http-proxy)=
## `juju-http-proxy`


`juju-http-proxy` is the HTTP proxy value to pass to charms in the `JUJU_CHARM_HTTP_PROXY` environment variable.

**Type:** string

**Default value:** ""


(model-config-juju-https-proxy)=
## `juju-https-proxy`

`juju-https-proxy` is the HTTPS proxy value to pass to charms in the `JUJU_CHARM_HTTPS_PROXY` environment variable.

**Type:** string

**Default value:** ""

(model-config-juju-no-proxy)=
## `juju-no-proxy`

`juju-no-proxy` is the list of domain addresses not to be proxied (comma-separated), may contain CIDRs. Passed to charms in the `JUJU_CHARM_NO_PROXY` environment variable.

**Type:** string

**Default value:** `127.0.0.1,localhost,::1`

**Valid values:**


## `logforward-enabled`

`logforward-enabled` determines whether syslog forwarding is enabled.

**Type:** boolean

**Default value:** false

(model-config-logging-config)=
## `logging-config`

`logging-config` is the configuration string to use when configuring Juju agent logging (see [this link](https://pkg.go.dev/github.com/juju/loggo#ParseConfigString) for details).

**Type:** string

**Value:** A (list of semicolon-separated) `<filter>=<verbosity level>` pairs,

where `<filter>` can be any of the following:

- `<root>` - matches all machine agent logs
- `unit` - matches all unit agent logs
- a module name, e.g. `juju.worker.apiserver`
<br><br>
A module represents a single component of Juju, e.g. a {ref}`worker <worker>`. Generally, modules correspond one-to-one with Go packages in the Juju source tree. The module name is the value passed to `loggo.GetLogger` or `loggo.GetLoggerWithLabels`.
<br><br>
Modules have a nested tree structure - for example, the `juju.api` module includes submodules `juju.api.application`, `juju.api.cloud`, etc. `<root>` is the root of this module tree.

- a label, e.g. `#charmhub`
<br><br>
*Labels* cut across the module tree, grouping various modules which deal with a certain feature or information flow. For example, the `#charmhub` label includes all modules involved in making a request to Charmhub.
<br><br>
The currently supported labels are:

| Label | Description |
|-|-|
| `#http` | HTTP requests |
| `#metrics` | Metric outputs - use as a fallback when Prometheus isn't available |
| `#charmhub` | Charmhub client and callers. |
| `#cmr` | Cross model relations |
| `#cmr-auth` | Authentication for cross model relations |
| `#secrets` | Juju secrets |

> See more: [https://github.com/juju/juju/blob/main/core/logger/labels.go](https://github.com/juju/juju/blob/main/core/logger/labels.go)

and where `<verbosity level>` can be, in decreasing order of severity:

| Level | Description |
|-|-|
| `CRITICAL` | Indicates a severe failure which could bring down the system. |
| `ERROR` | Indicates failure to complete a routine operation.
| `WARNING` | Indicates something is not as expected, but this is not necessarily going to cause an error.
| `INFO` | A regular log message intended for the user.
| `DEBUG` | Information intended to assist developers in debugging.
| `TRACE` | The lowest level - includes the full details of input args, return values, HTTP requests sent/received, etc. |

When you set `logging-config` to `module=level`, then Juju saves that module's logs for the given severity level **and above.** For example, setting `logging-config` to `juju.worker.uniter=WARNING` will capture all `CRITICAL`, `ERROR` and `WARNING` logs for the uniter, but discard logs for lower severity levels (`INFO`, `DEBUG`, `TRACE`).


> See more: [https://github.com/juju/loggo/blob/master/level.go#L13](https://github.com/juju/loggo/blob/master/level.go#L13)

**Examples:**

To collect debug logs for the `dbaccessor` worker:
```
juju model-config -m controller logging-config="juju.worker.dbaccessor=DEBUG"
```

To collect debug logs for the `mysql/0` unit:
```
juju model-config -m foo logging-config="unit.mysql/0=DEBUG"
```

To collect trace logs for Charmhub requests:
```
juju model-config -m controller logging-config="#charmhub=TRACE"
```

To see what API requests are being made:

```text
juju model-config -m controller logging-config="juju.apiserver=DEBUG"
```

To view details about each API request:

```text
juju model-config -m controller logging-config="juju.apiserver=TRACE"
```


## `logging-output`

`logging-output` is the logging output destination: database and/or syslog.

**Type:** string

**Default value:** ""

**Valid values:**

## `lxd-snap-channel`


`lxd-snap-channel` is the  channel to use when installing LXD from a snap (cosmic and later).

**Type:** string

**Valid values:** `latest`, `stable`

## `max-action-results-age`

`max-action-results-age` is the maximum age for action entries before they are pruned, in human-readable time format.

**Default value:** 336h

## `max-action-results-size`

`max-action-results-size` is the maximum size for the action collection, in human-readable memory format.

**Default value:** 5G

## `max-status-history-age`

`max-status-history-age` the maximum age for status history entries before they are pruned, in human-readable time format.

**Type:** string

**Default value:** 336h

**Valid values:** 72h, etc.


## `max-status-history-size`

`max-status-history-size` is the maximum size for the status history collection, in human-readable memory format.

**Type:** string

**Default value:** 5G

**Valid values:** 400M, 5G, etc.


## `net-bond-reconfigure-delay`

`net-bond-reconfigure-delay` is the amount of time in seconds to sleep between ifdown and ifup when bridging.

**Default value:** 17

(model-config-no-proxy)=
## `no-proxy`

`no-proxy` is the list of domain addresses not to be proxied (comma-separated).

**Type:** string

**Default value:** `127.0.0.1,localhost,::1`

## `num-container-provision-workers`

`num-container-provision-workers` is the number of container provisioning workers to use per machine.

**Default value:** 4


## `num-provision-workers`

`num-provision-workers` is the number of provisioning workers to use per model.

**Default value:** 16


## `provisioner-harvest-mode`

`provisioner-harvest-mode` sets what to do with unknown machines (default destroyed).

**Type:** string

**Default value:** `destroyed`

**Valid values:** `all`, `none`, `unknown`, `destroyed`

**Details:**

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

``` text
juju model-config provisioner-harvest-mode=none
```


## `proxy-ssh`

`proxy-ssh` determines whether SSH commands should be proxied through the API server.

**Type:** boolean

**Default value:** false


## `resource-tags`

`resource-tags` is a space-separated list of key=value pairs used to apply as tags on supported cloud models.

**Type:** string

**Default value:** none

(model-config-secret-backend)=
## `secret-backend`

`secret-backend` is the name of the secret store backend.

**Type:** string

**Default value:** `auto`

**Valid values:** `internal`, `auto`, `<>backend name`

(model-config-snap-http-proxy)=
## `snap-http-proxy`


`snap-http-proxy`  is the HTTP proxy value to for installing snaps.

**Type:** string

**Default value:** ""

(model-config-snap-https-proxy)=
## `snap-https-proxy`

`snap-https-proxy` is the snap-centric HTTPS proxy value.

**Type:** string

**Default value:** ""

(model-config-snap-store-assertions)=
## `snap-store-assertions`

`snap-store-assertions` is the HTTPS proxy value to for installing snaps.
**Type:** string

**Default value:** ""

(model-config-snap-store-proxy)=
## `snap-store-proxy`

`snap-store-proxy` is the snap store proxy for installing snaps.

**Type:** string

**Default value:** ""

(model-config-snap-store-proxy-url)=
## `snap-store-proxy-url`

`snap-store-proxy-url` is the URL for the defined snap store proxy.

**Type:** string

**Default value:** ""


## `ssl-hostname-verification`

`ssl-hostname-verification` determines whether SSL hostname verification is enabled.

**Type:** boolean

**Default value:** true

(storage-default-block-source)=
## `storage-default-block-source`

`storage-default-block-source`is the default block storage source for the model.

**Type:** string

**Default value:** -

**Valid values:** `loop` or the cloud-specific value

(storage-default-filesystem-source)=
## `storage-default-filesystem-source`

`storage-default-filesystem-source` is the default filesystem storage source for the model.

**Type:** string

**Default value:** -

**Valid values:** any storage provider (Juju will adjust)


## `transmit-vendor-metrics`

`transmit-vendor-metrics` determines whether metrics declared by charms deployed into this model are sent for anonymized aggregate analytics.

**Type:** boolean

**Default value:** true

## `update-status-hook-interval`

`update-status-hook-interval` sets how often to run the charm update-status hook, in human-readable time format (default 5m, range 1-60m).

**Type:** string

**Default value:** 5m

**Valid values:** 30s, 6m, 1hr, etc.
