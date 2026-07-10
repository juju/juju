---
myst:
  html_meta:
    description: "Use LXD cloud with Juju for local development, rapid prototyping, and testing. Learn localhost cloud setup, configuration, and use cases."
---

(cloud-lxd)=
# LXD

In Juju, [LXD](https://canonical.com/lxd) is a {ref}`machine cloud <machine-cloud>` that can run both system containers and virtual machines, and works as described below.

```{note}
This reference assumes basic familiarity with Juju. If you are new to Juju, start with the {ref}`Tutorial <tutorial>`, then use this page together with the generic materials it links to. For a cloud-specific starting point, see {ref}`lxd-appendix-example-workflows`.
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

(lxd-requirements)=
## Requirements

- Juju `2.9.x`: LXD `5.0`
- Juju `3.x.x`: LXD `5.x`

(lxd-concepts)=
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

As for all machine clouds, the cloud is registered in Juju via a cloud definition, stored in `clouds.yaml` on the client (on Linux: `~/.local/share/juju/clouds.yaml`) and following this schema:

```yaml
clouds:
  <cloud-name>:  # User-defined name
    type: lxd
    auth-types:
      - <auth-type>                # See Authentication types below
    endpoint: <lxd-api-url>        # LXD API endpoint (remote only)
    config:                        # Optional: model config defaults
      <config-key>: <value>        # See Configuration keys below
```


(lxd-cloud-localhost-vs-remote)=
### Localhost vs. remote LXD

- **Local LXD cloud**: Recognized as `localhost` cloud. Credential pre-defined for admin users.
- **Clustered LXD cloud**: In Juju, this counts as a remote cloud. You must add its definition to Juju explicitly.
- **Remote LXD cloud**: Requires the API endpoint URL for the remote LXD server.

(lxd-credential)=
## Credentials

```{ibnote}
See also: {ref}`credential`, {ref}`Juju | Manage credentials <manage-credentials>`, {ref}`Terraform Provider for Juju | Manage credentials <tfjuju:manage-credentials>`
```

As for all machine clouds, credentials are stored in `credentials.yaml` on the client and follow this schema:

```yaml
credentials:
  localhost                       # or user-defined for remote
    <credential-name>:             # User-defined credential name
      auth-type: <auth-type>       # certificate | interactive (see Authentication types below)
      <attribute>: <value>         # Auth-type-specific attributes (see below)
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

The controller runs on an LXD instance provisioned using the same mechanisms as workload machines -- see {ref}`lxd-machine-resources-created-per-machine` for the full per-machine resource model. Controller-specific differences are noted below.

**Compute**

- **LXD image**: Downloaded from image servers (simplestreams), filtered by base OS, architecture, and virtualization type, and cached locally in `/var/lib/lxd/images`.
- **Controller instance**: Created from the selected image with the `default` profile plus the model profile, connected to network and root storage, then started. Instance name: `juju-<modeluuid>-<machinenum>`.

**Storage**

- **Model profile**: Name `juju-<modelname>-<shortID>`. Includes settings such as `boot.autostart=true` and `security.nesting=true`. Applied to every container/VM in the model.

(lxd-model)=
## Models

```{ibnote}
See also: {ref}`model`, {ref}`Juju | Manage models <manage-models>`, {ref}`Terraform Provider for Juju | Manage models <tfjuju:manage-models>`
```

(lxd-model-configuration-keys)=
### Configuration keys

LXD supports the following {ref}`cloud-specific model configuration keys <model-config-cloud-specific-key>`:

**Storage**

(lxd-model-project)=
- **`project`**: The LXD project name to use for Juju's resources. Type: `string`. Default: `"default"`. Immutable: `false`.

(lxd-machine)=
## Machines

```{ibnote}
See also: {ref}`machine`, {ref}`Juju | Manage machines <manage-machines>`, {ref}`Terraform Provider for Juju | Manage machines <tfjuju:manage-machines>`
```

```{note}
With LXD system containers, constraints are interpreted as resource *maximums* (as opposed to *minimums*).

There is a 1:1 correspondence between a Juju machine and a LXD container/VM. Compare `juju machines` and `lxc list`.
```

(lxd-machine-constraints)=
### Constraints

LXD supports the following {ref}`constraints <constraint>`:

**Compute**

- {ref}`constraint-arch`. Valid values: Host architecture.
- {ref}`constraint-cores`
- {ref}`constraint-instance-type`
- {ref}`constraint-mem`. The maximum amount of memory that a machine/container will have.
- {ref}`constraint-virt-type`. Valid values: `container` (default), `virtual-machine`.

**Networking**

- {ref}`constraint-zones`. LXD node name(s). In clustered LXD, specifies which cluster member to place the instance on.

**Storage**

- {ref}`constraint-root-disk`
- {ref}`constraint-root-disk-source`. The LXD storage pool for the root disk. The default LXD storage pool is used if not specified.

(lxd-machine-placement-directives)=
### Placement directives

LXD supports the following {ref}`placement directives <placement-directive>`:

- {ref}`placement-directive-machine`
- {ref}`placement-directive-zone`: If there's no '=' delimiter, assume it's a node name.

(lxd-machine-resources-created-per-machine)=
### Resources created per machine

Applies to all machines, including controller machines. Controller-specific defaults are documented in {ref}`lxd-controller-resources-created-at-bootstrap`.

**Compute**

- **LXD instance**: Container (default) or VM (when constrained with `virt-type`). Name: `juju-<modeluuid>-<machinenum>`.
- **Profiles applied**: In order: (1) `default` (LXD built-in), (2) model profile (`juju-<model>-<id>`), (3) charm profiles (`juju-<model>-<id>-<appname>-<rev>`) if specified by charm.
- **Constraints via config**: `limits.cpu=<cores>` (CPU cores limit), `limits.memory=<MiB>MiB` (memory limit).

**Networking**

- **Network interfaces**: Default `eth0` bridged to default network. Additional NICs (`eth1`, `eth2`, etc.) for space constraints. Each NIC: type `nic`, `nictype=bridged`, parent host bridge, generated MAC address.
- **Cloud-init network config**: Netplan generated for multiple NICs when needed.

**Storage**

- **Root disk device** (if constraint specified): Type `disk`, pool from `root-disk-source`, path `/`, size in MiB.

(lxd-machine-networking-behavior)=
### Networking behavior

- **Network discovery**: Lists LXD networks and uses bridge networks for machine placement.
- **Subnet ID format**: `subnet-<hostBridgeName>-<CIDR>`. Example: `subnet-lxdbr0-10.0.0.0/24`.
- **NIC assignment**: Default `eth0` from `default` profile. Additional NICs for space constraints are bridged to host bridges.
- **IP assignment**: Assigned by host bridge DHCP on the LXD host.

(lxd-machine-storage-behavior)=
### Storage behavior

```{ibnote}
See also: {ref}`storage-provider-lxd` for the LXD storage provider configuration options.
```

- **Storage pools**: Juju storage pool creates a corresponding LXD storage pool. LXD pool for Juju `lxd` pool created on first use (named `juju`).
- **Volumes stored**: `/var/lib/lxd/storage-pools/<pool-name>`.
- **Default pools attempted**: `lxd-zfs` (driver `zfs`) then `lxd-btrfs` (driver `btrfs`).

(lxd-machine-charm-profiles)=
### Charm profiles

LXD Profiles allow a configuration to be applied to any instance. Juju applies charm profiles during the creation or modification of a LXD container.

```{ibnote}
See more: [Charmcraft | `lxd-profile.yaml`](https://canonical-charmcraft.readthedocs-hosted.com/stable/reference/files/lxd-profile-yaml-file/)
```

(lxd-storage)=
## Storage

```{ibnote}
See also: {ref}`storage`, {ref}`Juju | Manage storage <manage-storage>`
```

(lxd-storage-providers)=
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

**Example deployment:**

```bash
juju deploy postgresql --storage pgdata=lxd,8G
```

(lxd-appendix-example-workflows)=
## Appendix: Example workflows

(lxd-appendix-remote-bootstrap)=
### Bootstrap on a remote LXD server

1. Add the remote LXD cloud with `juju add-cloud`, providing the remote server's API endpoint URL.
2. Add credentials with `juju add-credential` choosing `interactive` or `certificate`.
3. Bootstrap with `juju bootstrap <cloud-name> <controller-name>`.

(lxd-appendix-clustering)=
### LXD clustering

LXD clustering provides high-availability deployment. In a clustered LXD cloud, Juju deploys units across cluster nodes. Availability zones map to cluster member names.

```{ibnote}
See more: [LXD | Clustering](https://documentation.ubuntu.com/lxd/stable-5.21/clustering)
```

(lxd-appendix-projects)=
### LXD projects

LXD projects provide isolated namespaces for models (multi-tenancy). Configured via cloud spec `Project` field. Profile, network, storage, and container APIs are scoped to the project.

(lxd-appendix-images)=
### LXD images

LXD is image-based. All LXD containers come from images and any LXD daemon instance (also called a "remote") can serve images. When LXD is installed, a locally-running remote is provided (Unix domain socket) and the client is configured to talk to it (named 'local'). The client is also configured to talk to several other, non-local, ones (named 'ubuntu', 'ubuntu-daily', and 'images').

An image is identified by its fingerprint (SHA-256 hash), and can be tagged with multiple aliases. Juju pulls official cloud images from the 'ubuntu' remote (http://cloud-images.ubuntu.com) and caches them locally in `/var/lib/lxd/images`.
