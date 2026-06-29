---
myst:
  html_meta:
    description: "Install, configure, and manage the Juju CLI client on Linux and macOS. Learn to handle authentication, upgrades, and plugins."
---

(manage-juju)=
# How to manage the `juju` CLI client

```{ibnote}
See also: {ref}`juju-cli`
```

(install-juju)=
## Install `juju`

``````{tabs}

`````{tab} Linux

**Install from snap.**

```{note}
**Why install from snap?** Snaps get updated automatically. Thus, your client will be updated automatically as soon as a new Juju release becomes available.

**Snap command not available on your system?** Visit [snapcraft.io](https://snapcraft.io) for instructions on how to install `snapd`.

```

To install `juju` from snap, run:

```text
sudo snap install juju
```

To select a particular version, run `snap info juju` to find out what versions are available, then `sudo snap install juju --channel=<track/risk[/branch]>` to install the version of your choice (e.g., `sudo snap install juju --channel=3.4/stable`).

````{dropdown} Example

```text
$ snap info juju
name:    juju
summary: Juju - a model-driven operator lifecycle manager for K8s and
  machines
publisher: Canonical✓
store-url: https://snapcraft.io/juju
contact:   https://canonical.com/
license:   AGPL-3.0
description: |
  A model-driven **universal operator lifecycle manager** for multi cloud and
  hybrid cloud application management on K8s and machines.

  **What is an operator lifecycle manager?**
  Kubernetes operators are containers with operations code, that drive your
  applications on K8s. Juju is an operator lifecycle manager that manages the
  installation, integration and configuration of operators on the cluster.
  Juju also extends the idea of operators to traditional application
  management on Linux servers, or cloud instances.

  **Model-driven operations and integration**
  Organise your operators into models, which group together applications that
  can be tightly integrated on the same substrate and operated by the same
  team. Capture resource allocation, storage, networking and integration
  information in the model to simplify ongoing operations.

  **Better day-2 operations**
  Each operator code package, called a charm, declares methods for actions
  like back, restore, or security audit. Calling these methods provides
  remote administration of the application with no low-level access required.

  **Learn more**

   - https://juju.is/
   - https://discourse.charmhub.io/
   - https://github.com/juju/juju
commands:
  - juju
services:
  juju.fetch-oci: oneshot, disabled, inactive
snap-id:      e2CPHpB1fUxcKtCyJTsm5t3hN9axJ0yj
tracking:     3.1/stable
refresh-date: 2024-01-03
channels:
  3/stable:      3.4.0             2024-03-07 (26548)  99MB -
  3/candidate:   ↑
  3/beta:        ↑
  3/edge:        ↑
  4.0/stable:    –
  4.0/candidate: –
  4.0/beta:      4.0-beta2         2024-01-11 (25984)  98MB -
  4.0/edge:      4.0-beta3-ec9b93b 2024-02-19 (26600)  98MB -
  4/stable:      –
  4/candidate:   –
  4/beta:        4.0-beta2         2024-01-17 (25984)  98MB -
  4/edge:        ↑
  3.5/stable:    –
  3.5/candidate: –
  3.5/beta:      –
  3.5/edge:      3.5-beta1-c3de749 2024-03-12 (26766)  98MB -
  3.4/stable:    3.4.0             2024-02-15 (26548)  99MB -
  3.4/candidate: ↑
  3.4/beta:      ↑
  3.4/edge:      3.4.1-14d5608     2024-03-13 (26783)  98MB -
  3.3/stable:    3.3.3             2024-03-06 (26652)  99MB -
  3.3/candidate: ↑
  3.3/beta:      ↑
  3.3/edge:      3.3.4-65b78cd     2024-03-13 (26779)  99MB -
  3.2/stable:    3.2.4             2023-11-22 (25443)  95MB -
  3.2/candidate: ↑
  3.2/beta:      ↑
  3.2/edge:      3.2.5-9e20221     2023-11-17 (25455)  95MB -
  3.1/stable:    3.1.7             2024-01-03 (25751)  95MB -
  3.1/candidate: ↑
  3.1/beta:      ↑
  3.1/edge:      3.1.8-1a8d6a3     2024-03-12 (26750)  95MB -
  2.9/stable:    2.9.46            2023-12-05 (25672) 120MB classic
  2.9/candidate: 2.9.47            2024-03-07 (26724) 120MB classic
  2.9/beta:      ↑
  2.9/edge:      2.9.48-dfd7fee    2024-03-07 (26740) 120MB classic
  2.8/stable:    2.8.13            2021-11-11 (17665)  74MB classic
  2.8/candidate: ↑
  2.8/beta:      ↑
  2.8/edge:      ↑
installed:       3.1.7                        (25751)  95MB -

$ sudo snap install juju --channel=3.4/stable


```

````

To install multiple versions of `juju` via snap, enable `snap`'s experimental parallel-install feature, reboot, then install a different version with a different name.

```{ibnote}
See more: [Snap | Channels](https://snapcraft.io/docs/channels)
```

````{dropdown} Example

```text

# Enable snap's experimental parallel-install feature:
sudo snap set system experimental.parallel-instances=true`

# Reboot.

# Install juju 2.9 under the name 'juju_29'
sudo snap install --channel 2.9/stable juju_29 --classic

# Install juju 3.3 under the name 'juju_33'
sudo snap install --channel 3.3/stable juju_33

# Test your 2.9 client:
juju_29 status

# Test your 3.3 client:
juju_33 status


```
````

```{ibnote}
See more: [Snap | Parallel installs](https://snapcraft.io/docs/parallel-installs)
```

**Install from binary.**

This method allows you to install the Juju client on systems that do not support snaps.

1. Visit the project's [releases](https://github.com/juju/juju/releases) page, select the version you want to install, and expand the **Assets** dropdown to see the available binaries. Download the binary that matches your system's architecture.

For example, to download the 3.6.23 client for amd64:

```text
curl -LO https://github.com/juju/juju/releases/download/v3.6.23/juju-3.6.23-linux-amd64.tar.xz
```

2. Extract and install the binary

```text
tar xf juju-3.6.23-linux-amd64.tar.xz
sudo install -o root -g root -m 0755 juju /usr/local/bin/juju
```

3. Verify the installation

```text
juju version
```

**Build from source.** Visit the [releases](https://github.com/juju/juju/releases) page to download a tar.gz with Juju source code. For build instructions refer to the [contributing to Juju](https://github.com/juju/juju/blob/main/CONTRIBUTING.md) documentation on Github.


`````

`````{tab} macOS

The Juju client is available on [Homebrew](https://brew.sh/) and can be installed as follows:

```text
brew install juju
```

`````

``````

## Use `juju`


Use the `juju` CLI client reference and the Juju how-to guides to build up your deployment.

```{ibnote}
See more: {ref}`command-juju-help`, {ref}`List of juju CLI commands <list-of-juju-cli-commands>`, {ref}`how-to-guides`
```


## Back up `juju`


Backing up your `juju` client enables you to regain management control of your controllers and associated cloud environments.

**Create a backup of the `juju` client.** To create a backup of your `juju` client, make a copy of the client directory. This is normally done with backup software that compresses the data into a single file (archive). For example, on a Linux/Ubuntu system, you could use `tar`:

``` text
cd ~
tar -cpzf juju-client-$(date "+%Y%m%d-%H%M%S").tar.gz .local/share/juju
```

The above invocation embeds a timestamp in the generated archive's filename, which is useful for knowing **when** a backup was made. You may, of course, call it whatever you wish.

Once the backup file is ready, remember: Whoever has access to a client backup will have access to its associated environments.  Thus, take measures to keep it safe (e.g., transfer it to another system, encrypt it, etc.).

**Restore the `juju` client from a backup.** To restore your client from a backup, extract the backup created earlier. E.g., on Ubuntu:

```{important}

This command will extract the contents of the archive and overwrite any existing files in the Juju directory. Make sure that this is what you want.

```

``` text
cd ~
tar -xzf juju-yymmdd-hhmmss.tar.gz
```

(upgrade-juju)=
## Upgrade `juju`

```{ibnote}
See also: {ref}`upgrading-things`
```

``````{tabs}

`````{tab} Linux

**If you've installed via `snap`.**

If the Juju client was installed via snap, the updates to the client should be handled automatically. Run `snap info juju` to view a list of releases and `juju version` to view the current release.

If there has been a new release but the `juju` snap hasn't been refreshed, you can manually trigger this with `sudo snap refresh juju`. To refresh to a specific version, run the `refresh` command with the `--channel=<track/version>` option, e.g.

```text
sudo snap refresh juju --channel 3/stable
```

```{ibnote}
See more: [Snap | Managing updates](https://snapcraft.io/docs/managing-updates), [Snap | Channels](https://snapcraft.io/docs/channels)
```

`````

`````{tab} macOS

To upgrade Juju to the latest stable release, run
```text
brew upgrade juju

`````
``````

## Uninstall `juju`

``````{tabs}

`````{tab} Linux

**If you've installed `juju` via `snap`:** To uninstall, run:

```text
sudo snap remove juju
```

`````

`````{tab} macOS

```text
brew uninstall juju
```

`````
``````

