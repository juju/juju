---
myst:
  html_meta:
    description: "Use LXD cloud with Juju for local development, rapid prototyping, and testing. Learn localhost cloud setup, configuration, and use cases."
---

(cloud-lxd)=
# LXD

In Juju, [LXD](https://ubuntu.com/lxd) is a {ref}`machine cloud <machine-cloud>` that can run both system containers and virtual machines. It behaves like all machine clouds, except for a few points of variation related to the cloud, credentials, controllers, models, machines, and storage, described below.

```{note}
This reference assumes basic familiarity with Juju. If you are new to Juju, start with the {ref}`tutorial`, then use this page together with the generic materials it links to. For a cloud-specific starting point, see {ref}`lxd-appendix-example-workflows`.
```

```{dropdown} Reasons to use a LXD cloud

The LXD cloud, especially when used locally, is great for:

- creating a repeatable deployment: Juju enables you to quickly iterate to construct the optimal deployment for your situation, then distribute that across your team.
- local development: Juju's localhost cloud can mirror the production ops environment (without incurring the costs involved with duplicating it).
- learning Juju: LXD is a lightweight tool for exploring Juju and how it operates.
- rapid prototyping: LXD is great for when you're creating a new charm and want to be able to quickly provision capacity and tear it down.
```

```{dropdown} Why Docker wouldn't work

Juju expects to see an operating system-like environment, so a LXD system container fits the bill. Docker containers are laid out for a singular application process, with a self-contained filesystem rather than a base userspace image.
```

(lxd-cloud-requirements)=
## Requirements

**Juju version compatibility:**

- Juju `2.9.x`: LXD `5.0`
- Juju `3.x.x`: LXD `5.x`

(lxd-cloud-concepts)=
## Concepts

The following table shows how LXD abstractions map to Juju concepts:

| LXD | Juju |
| - | - |
| Container or VM instance | {ref}`machine <machine>` |
| Process in a container or VM | {ref}`unit <unit>` |
| Group of units for one workload | {ref}`application <application>` |
| Storage pool volume | {ref}`storage <storage>` |
| LXD project | Administrative boundary for model resources (roughly) |
| Cluster member | Placement target (`zones`) |

(lxd-cloud)=
## The cloud

```{ibnote}
See also: {ref}`cloud`, {ref}`Juju | Manage clouds <manage-clouds>`, {ref}`Terraform Provider for Juju | Manage clouds <tfjuju:manage-clouds>`
```

Type in Juju: `lxd`

Name in Juju: `localhost` (for local LXD), user-defined (for remote/clustered LXD)

(lxd-cloud-other)=
### Other

(lxd-cloud-localhost-vs-remote)=
#### Localhost vs. remote LXD

- **Local LXD cloud**: Recognized as `localhost` cloud. Credential pre-defined for admin users.
- **Clustered LXD cloud**: In Juju, this counts as a remote cloud. You must add its definition to Juju explicitly.
- **Remote LXD cloud**: Requires the API endpoint URL for the remote LXD server.

(lxd-cloud-clustering)=
#### LXD clustering

LXD clustering provides high-availability deployment. In a clustered LXD cloud, Juju deploys units across cluster nodes. Availability zones map to cluster member names.

```{ibnote}
See more: [LXD | Clustering](https://documentation.ubuntu.com/lxd/stable-5.21/clustering)
```

(lxd-cloud-projects)=
#### LXD projects

LXD projects provide isolated namespaces for models (multi-tenancy). Configured via cloud spec `Project` field. Profile, network, storage, and container APIs are scoped to the project.

(lxd-cloud-images)=
#### LXD images

LXD is image based: All LXD containers come from images and any LXD daemon instance (also called a "remote") can serve images. When LXD is installed, a locally-running remote is provided (Unix domain socket) and the client is configured to talk to it (named 'local'). The client is also configured to talk to several other, non-local, ones (named 'ubuntu', 'ubuntu-daily', and 'images').

An image is identified by its fingerprint (SHA-256 hash), and can be tagged with multiple aliases.

For any image-related command, an image is specified by its alias or by its fingerprint. Both are shown in image lists. An image's *filename* is its *full* fingerprint, while an image *list* displays its *partial* fingerprint. Either type of fingerprint can be used to refer to images.

Juju pulls official cloud images from the 'ubuntu' remote (http://cloud-images.ubuntu.com) and creates the necessary alias. Any subsequent requests will be satisfied by the LXD cache (`/var/lib/lxd/images`).

Image cache expiration and image synchronization mechanisms are built-in.

(lxd-credential)=
## Credentials

```{ibnote}
See also: {ref}`credential`, {ref}`Juju | Manage credentials <manage-credentials>`, {ref}`Terraform Provider for Juju | Manage credentials <tfjuju:manage-credentials>`
```

**Local LXD cloud:** If you are a Juju admin user, the credential is already known to Juju. Run `juju bootstrap`, then `juju credentials` to confirm. (Pre-defined credential name in Juju: `localhost`.) Otherwise, add manually as you would a remote.

**Remote LXD cloud:** Requires the API endpoint URL for the remote LXD server.

```{ibnote}
See more: [LXD | How to add remote servers](https://documentation.ubuntu.com/lxd/en/latest/remotes/), {ref}`lxd-appendix-remote-bootstrap`
```

(lxd-credential-authentication-types)=
### Authentication types

LXD supports the following authentication types:

(lxd-credential-certificate)=
#### `certificate`

Attributes:

- `server-cert`: The path to the PEM-encoded LXD server certificate file (required).
- `client-cert`: The path to the PEM-encoded LXD client certificate file (required).
- `client-key`: The path to the PEM-encoded LXD client key file (required).

(lxd-credential-interactive)=
#### `interactive`

Attributes:

- `trust-token`: The LXD server trust token (optional, required if `trust-password` is not set). This is the recommended method for authenticating with a remote LXD server. Added in Juju 3.6.4.
- `trust-password`: The LXD server trust password (optional, required if `trust-token` is not set).

```{ibnote}
See more: [LXD | Adding client certificates using tokens](https://documentation.ubuntu.com/lxd/en/stable-5.0/authentication/#adding-client-certificates-using-tokens)
```

(lxd-controller)=
## Controllers

```{ibnote}
See also: {ref}`controller`, {ref}`Juju | Manage controllers <manage-controllers>`, {ref}`Terraform Provider for Juju | Manage controllers <tfjuju:manage-controllers>`
```

(lxd-controller-bootstrap-behavior)=
### Bootstrap behavior

Creates a controller container or VM on LXD. Uses LXD API calls to create profiles, images, and instances.

```{note}
If `juju bootstrap` hangs, it could be due to a firewall issue. See: [LXD | UFW: Add rules for the bridge](https://documentation.ubuntu.com/lxd/latest/howto/network_bridge_firewalld/#ufw-add-rules-for-the-bridge).
```

(lxd-controller-resources-created-at-bootstrap)=
### Resources created at bootstrap

- **Model profile**: Name `juju-<modelname>-<shortID>`. Includes settings such as `boot.autostart=true` and `security.nesting=true`. Applied to every container/VM in the model.
- **LXD image**: Downloaded from image servers (simplestreams), filtered by base OS, architecture, and virtualization type, and cached locally in `/var/lib/lxd/images`.
- **Controller instance**: Created from the selected image with the `default` profile plus the model profile, connected to network and root storage, then started. Instance name: `juju-<modeluuid>-<machinenum>`.

(lxd-model)=
## Models

```{ibnote}
See also: {ref}`model`, {ref}`Juju | Manage models <manage-models>`, {ref}`Terraform Provider for Juju | Manage models <tfjuju:manage-models>`
```

(lxd-model-configuration-keys)=
### Configuration keys

LXD supports the following {ref}`cloud-specific model configuration keys <model-config-cloud-specific-key>`:

(lxd-model-project)=
#### `project`

The LXD project name to use for Juju's resources.

- **Type**: `string`
- **Default value**: `"default"`
- **Immutable**: `false`
- **Mandatory**: `false`

(lxd-machine)=
## Machines

```{ibnote}
See also: {ref}`machine`, {ref}`Juju | Manage machines <manage-machines>`, {ref}`Terraform Provider for Juju | Manage machines <tfjuju:manage-machines>`
```

When provisioning machines on LXD, Juju supports the following constraints and placement directives.

```{note}
With LXD system containers, constraints are interpreted as resource *maximums* (as opposed to *minimums*).

There is a 1:1 correspondence between a Juju machine and a LXD container/VM. Compare `juju machines` and `lxc list`.
```

(lxd-machine-constraints)=
### Constraints

LXD supports the following {ref}`constraints <constraint>`:

- {ref}`constraint-arch`. Valid values: Host architecture.
- {ref}`constraint-cores`
- {ref}`constraint-instance-type`
- {ref}`constraint-mem`. The maximum amount of memory that a machine/container will have.
- {ref}`constraint-root-disk`
- {ref}`constraint-root-disk-source`. The LXD storage pool for the root disk. The default LXD storage pool is used if not specified.
- {ref}`constraint-virt-type`. Valid values: `container` (default), `virtual-machine`.
- {ref}`constraint-zones`. LXD node name(s). In clustered LXD, specifies which cluster member to place the instance on.

(lxd-machine-placement-directives)=
### Placement directives

LXD supports the following placement directives:

- {ref}`placement-directive-machine`
- {ref}`placement-directive-zone`: If there's no '=' delimiter, assume it's a node name.

(lxd-machine-resources-created-per-machine)=
### Resources created per machine

Each machine (controller or application) receives:

- **LXD instance**: Container (default) or VM (when constrained with `virt-type`). Name: `juju-<modeluuid>-<machinenum>`.
- **Profiles applied**: In order: (1) `default` (LXD built-in), (2) model profile (`juju-<model>-<id>`), (3) charm profiles (`juju-<model>-<id>-<appname>-<rev>`) if specified by charm.
- **Constraints via config**: `limits.cpu=<cores>` (CPU cores limit), `limits.memory=<MiB>MiB` (memory limit).
- **Root disk device** (if constraint specified): Type `disk`, pool from `root-disk-source`, path `/`, size in MiB.
- **Network interfaces**: Default `eth0` bridged to default network. Additional NICs (`eth1`, `eth2`, etc.) for space constraints. Each NIC: type `nic`, `nictype=bridged`, parent host bridge, generated MAC address.
- **Cloud-init network config**: Netplan generated for multiple NICs when needed.

(lxd-machine-networking-behavior)=
### Networking behavior

- **Network discovery**: Lists LXD networks and uses bridge networks for machine placement.
- **Subnet ID format**: `subnet-<hostBridgeName>-<CIDR>`. Example: `subnet-lxdbr0-10.0.0.0/24`.
- **NIC assignment**: Default `eth0` from `default` profile. Additional NICs for space constraints are bridged to host bridges.
- **IP assignment**: Assigned by host bridge DHCP on the LXD host.

(lxd-machine-other)=
### Other

(lxd-machine-charm-profiles)=
#### Charm profiles

LXD Profiles allows the definition of a configuration that can be applied to any instance. Juju can apply those profiles during the creation or modification of a LXD container.

```{ibnote}
See more: [Charmcraft | `lxd-profile.yaml`](https://canonical-charmcraft.readthedocs-hosted.com/stable/reference/files/lxd-profile-yaml-file/)
```

(lxd-storage)=
## Storage

```{ibnote}
See also: {ref}`storage`, {ref}`Juju | Manage storage <manage-storage>`
```

### Storage providers

In addition to generic storage providers, LXD provides the following {ref}`cloud-specific storage providers <storage-provider-cloud-specific>`:

(storage-provider-lxd)=
#### `lxd`

**Type:** LXD storage pools (filesystem-backed, no volumes)

**Scope:** Environment-wide pools

**Configuration options:**

- `driver`: LXD storage driver. Valid values: `zfs`, `btrfs`, `lvm`, `ceph`, `dir`.
- `lxd-pool`: The name to give to the corresponding storage pool in LXD.

Any other parameters will be passed to LXD (e.g., `zfs.pool_name`).

```{ibnote}
See more: [LXD storage configuration](https://documentation.ubuntu.com/lxd/latest/reference/storage_drivers/)
```

**Default pools attempted:**

1. `lxd-zfs`: Driver `zfs`, pool `juju-zfs`, `zfs.pool_name=juju-lxd`
2. `lxd-btrfs`: Driver `btrfs`, pool `juju-btrfs`

**Behavior:**

- Juju storage pool creates corresponding LXD storage pool.
- LXD pool for Juju `lxd` pool created on first use (named `juju`).
- Volumes stored in LXD pool: `/var/lib/lxd/storage-pools/<pool-name>`.
- Use `lxc storage list` to list LXD storage pools.

**Example deployment:**

```bash
juju deploy postgresql --storage pgdata=lxd,8G
```

(lxd-appendix-example-workflows)=
## Appendix: Example workflows

(lxd-appendix-quickstart)=
### Add cloud, add credential, bootstrap

On a local development host, LXD `localhost` is typically pre-defined in Juju. Configure LXD, confirm cloud/credential visibility, then bootstrap.

```text
lxd init --auto
lxc network set lxdbr0 ipv6.address none
juju clouds --client
juju credentials
juju bootstrap localhost lxd-controller
```

(lxd-appendix-remote-bootstrap)=
### Add a remote LXD cloud and bootstrap

From Juju 2.9.5, the easiest method for bootstrapping a remote LXD server is to add the remote to your local LXC config then bootstrap with `juju`.

**On the remote server:**

```bash
# Ensure the LXD daemon is listening on an accessible IP
lxc config set core.https_address '[::]'
# Give the LXD daemon a trust password so the client can register credentials
lxc config set core.trust_password mytrustpassword
```

**On the bootstrapping client:**

```bash
# Add the remote LXD server to the local LXC config
lxc remote add myremote 11.22.33.44 --password mytrustpassword
# Bootstrap juju using the remote name in LXC
juju bootstrap myremote
```

```{note}
The bootstrapping client must be able to reach the remote LXD containers. This may require the setup of a bridge device with the host's ethernet device.
```

(lxd-appendix-loop-devices)=
## Appendix: Loop devices and LXD

LXD (localhost) does not officially support attaching loopback devices for storage out of the box. However, with some configuration you can make this work.

Each container uses the 'default' LXD profile, but also uses a model-specific profile with the name `juju-<model-name>-<model-short-UUID>` where `<model-short-UUID>` is the first 6 characters of the model UUID. Editing a profile will affect all of the containers using it, so you can add loop devices to all LXD containers by editing the 'default' profile, or you can scope it to a model.

To add loop devices to your container, add entries to the 'default', or model-specific, profile, with `lxc profile edit <profile>`:

```yaml
...
devices:
  loop-control:
    major: "10"
    minor: "237"
    path: /dev/loop-control
    type: unix-char
  loop0:
    major: "7"
    minor: "0"
    path: /dev/loop0
    type: unix-block
  loop1:
    major: "7"
    minor: "1"
    path: /dev/loop1
    type: unix-block
...
  loop9:
    major: "7"
    minor: "9"
    path: /dev/loop9
    type: unix-block
```

Doing so will expose the loop devices so the container can acquire them via the `losetup` command. However, it is not sufficient to enable the container to mount filesystems onto the loop devices. One way to achieve that is to make the container "privileged" by adding:

```yaml
config:
  security.privileged: "true"
```


