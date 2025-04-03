(manage-juju)=
# How to manage the `juju` CLI client

> See also: {ref}`juju-cli`


## Install `juju`

``````{tabs}

`````{tab} Linux

**Install from snap.**

```{important}
**Why install from snap?** Snaps get updated automatically. Thus, your client will be updated automatically as soon as a new Juju release becomes available.

**Snap command not available on your system?** Visit [snapcraft.io](https://snapcraft.io) for instructions on how to install `snapd`.

```

To install `juju` from snap, run:

```text
sudo snap install juju
```

To select a particular version, run `snap info juju` to find out what versions are available, then `sudo snap install juju --channel=<track/risk[/branch]>` to install the version of your choice (e.g., `sudo snap install juju --channel=2.9/stable`).

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
  management on Linux and Windows servers, or cloud instances.

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
snap-id: e2CPHpB1fUxcKtCyJTsm5t3hN9axJ0yj
channels:
  3/stable:      3.6.4             2025-03-11 (30244) 103MB -
  3/candidate:   ↑
  3/beta:        3.6-beta2         2024-08-02 (28096) 100MB -
  3/edge:        ↑
  4.0/stable:    –
  4.0/candidate: –
  4.0/beta:      4.0-beta5         2025-03-13 (30332)  99MB -
  4.0/edge:      4.0-beta6-0feb309 2025-04-03 (30682) 102MB -
  4/stable:      –
  4/candidate:   –
  4/beta:        4.0-beta5         2025-03-13 (30332)  99MB -
  4/edge:        ↑
  3.6/stable:    3.6.4             2025-03-11 (30244) 103MB -
  3.6/candidate: ↑
  3.6/beta:      3.6-beta2         2024-08-02 (28096) 100MB -
  3.6/edge:      3.6.5-9dbf2a8     2025-04-02 (30670) 105MB -
  3.5/stable:    3.5.7             2025-03-11 (30149) 103MB -
  3.5/candidate: ↑
  3.5/beta:      ↑
  3.5/edge:      3.5.8-d43eb4d     2025-03-24 (30495) 103MB -
  3.4/stable:    3.4.6             2024-10-01 (28491)  98MB -
  3.4/candidate: ↑
  3.4/beta:      ↑
  3.4/edge:      3.4.7-20a2e39     2025-02-28 (30115) 101MB -
  3.3/stable:    3.3.7             2024-10-01 (28471)  99MB -
  3.3/candidate: ↑
  3.3/beta:      ↑
  3.3/edge:      3.3.8-f5de3be     2024-11-11 (28920)  99MB -
  3.2/stable:    3.2.4             2023-11-22 (25443)  95MB -
  3.2/candidate: ↑
  3.2/beta:      ↑
  3.2/edge:      3.2.5-9e20221     2023-11-17 (25455)  95MB -
  3.1/stable:    3.1.10            2024-10-01 (28463)  95MB -
  3.1/candidate: ↑
  3.1/beta:      ↑
  3.1/edge:      3.1.11-07dba11    2025-02-26 (30044)  98MB -
  2.9/stable:    2.9.51            2024-10-01 (28362) 120MB classic
  2.9/candidate: ↑
  2.9/beta:      ↑
  2.9/edge:      2.9.52-edea7a8    2025-03-28 (30601) 123MB classic
  2.8/stable:    2.8.13            2021-11-11 (17665)  74MB classic
  2.8/candidate: ↑
  2.8/beta:      ↑
  2.8/edge:      ↑

$ sudo snap install juju --channel=2.9/stable


```

````

To install multiple versions of `juju` via snap, enable `snap`'s experimental parallel-install feature, reboot, then install a different version with a different name.

> See more: [Snap | Channels](https://snapcraft.io/docs/channels)

````{dropdown} Example

```text

# Enable snap's experimental parallel-install feature:
sudo snap set system experimental.parallel-instances=true`

# Reboot.

# Install juju 2.9 under the name 'juju_29'
sudo snap install --channel 2.9/stable juju_29 --classic

# Install juju 3.6 under the name 'juju_36'
sudo snap install --channel 3.6/stable juju_36

# Test your 2.9 client:
juju_29 status

# Test your 3.6 client:
juju_36 status


```
````

> See more: [Snap | Parallel installs](https://snapcraft.io/docs/parallel-installs)

**Install from binary.**

This method allows you to install the Juju client on systems that do not support snaps.

1. Visit the project's [downloads](https://launchpad.net/juju/+download) page and select the binary that matches your system's architecture and the version that you want to install.

For example, to download the 2.9.38 client for amd64:

```text
curl -LO https://launchpad.net/juju/2.9/2.9.38/+download/juju-2.9.38-linux-amd64.tar.xz
```

2. Validate the downloaded binary archive (optional)

Download the md5 checksum that matches the binary you just downloaded:

```{note}

The link to the `md5` signature can be constructed by appending `/+md5` to the end of the link you just downloaded.

```

```text
curl -L https://launchpad.net/juju/2.9/2.9.38/+download/juju-2.9.38-linux-amd64.tar.xz/+md5 -o juju.md5
```

Validate the downloaded binary archive against the checksum file:

```text
cat juju.md5 | md5sum --check
```

If the checksum check succeeds, the output will be:

```text
juju-2.9.38-linux-amd64.tar.xz: OK
```

If the check fails, md5sum exits with nonzero status and prints output similar to:

```text
juju-2.9.38-linux-amd64.tar.xz: FAILED
md5sum: WARNING: 1 computed checksum did NOT match
```

3. Unpack and install client binary

```text
tar xf juju-2.9.38-linux-amd64.tar.xz
sudo install -o root -g root -m 0755 juju /usr/local/bin/juju
```

4. Test that the version of the client you installed is up to date

```text
juju version
```

**Build from source.** Visit the [downloads section](https://launchpad.net/juju/+download) of the [Launchpad project](https://launchpad.net/juju/) to download a tar.gz with Juju source code. For build instructions refer to the [contributing to Juju](https://github.com/juju/juju/blob/develop/CONTRIBUTING.md) documentation on Github.


`````

`````{tab} macOS

The Juju client is available on [Homebrew](https://brew.sh/) and can be installed as follows:

```text
brew install juju
```

`````

`````{tab} Windows

Visit the project's [downloads](https://launchpad.net/juju/+download) page and select the signed installer for the Juju version you wish to install.

`````

``````

## Use `juju`


Use the `juju` CLI client reference and the Juju how-to guides to build up your deployment.

> See more: {ref}`command-juju-help`, {ref}`how-to-guides`


## Back up Juju

```{note}
A backup of the client enables one to regain management control of one's controllers and associated cloud environments.
```

**Create a backup of the `juju` client.** Making a copy of the client directory is sufficient for backing up the client. This is normally done with backup software that compresses the data into a single file (archive). On a Linux/Ubuntu system, the `tar` program is a common choice:

``` text
cd ~
tar -cpzf juju-client-$(date "+%Y%m%d-%H%M%S").tar.gz .local/share/juju
```

```{note}

For Microsoft Windows any native Windows backup tool will do.

```

The above invocation embeds a timestamp in the generated archive's filename, which is useful for knowing **when** a backup was made. You may, of course, call it what you wish.

The archive should normally be transferred to another system (or at the very least to a different physical drive) for safe-keeping.

```{note}

Whoever has access to a client backup will have access to its associated environments. Appropriate steps should be taken to protect it (e.g. encryption).

```

**Restore the `juju` client from a backup.** To restore your client from a backup, extract the backup created earlier. E.g., on Ubuntu:

```{note}

This command will extract the contents of the archive and overwrite any existing files in the Juju directory. Make sure that this is what you want.

```

``` text
cd ~
tar -xzf juju-yymmdd-hhmmss.tar.gz
```

(upgrade-juju)=
## Upgrade `juju`

> See also: {ref}`upgrading-things`

``````{tabs}

`````{tab} Linux

**If you've installed via `snap`.**

```{note}
Ensure you've created a backup of your ./local/share/juju before starting the upgrade process for the client.
```

If the Juju client was installed via snap, the updates to the client should be handled automatically. Run `snap info juju` to view a list of releases and `juju version` to view the current release.

If there has been a new release but the `juju` snap hasn't been refreshed, you can manually trigger this with `sudo snap refresh juju`. To refresh to a specific version, run the `refresh` command with the `--channel=<track/version>` option, e.g.

```text
sudo snap refresh juju --channel 3/stable
```

> See more: [Snap | Managing updates](https://snapcraft.io/docs/managing-updates), [Snap | Channels](https://snapcraft.io/docs/channels)

`````

`````{tab} macOS

To upgrade Juju to the latest stable release, run
```text
brew upgrade juju

`````

`````{tab} Windows

Visit the project's [downloads](https://launchpad.net/juju/+download) page and select the signed installer for the latest stable version of Juju you wish to install.

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

`````{tab} Windows

Uninstall the juju client application using your system's application management settings.

`````
``````

