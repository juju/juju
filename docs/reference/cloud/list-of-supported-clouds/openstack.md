---
myst:
  html_meta:
    description: "Deploy on OpenStack cloud using Juju, including supported versions, novarc file configuration, and authentication with Keystone."
---

(cloud-openstack)=
# OpenStack

In Juju, [OpenStack](https://www.openstack.org/software/) is a {ref}`machine cloud <machine-cloud>` and works as described below.

```{note}
This reference assumes basic familiarity with Juju. If you are new to Juju, start with the {ref}`Tutorial <tutorial>`, then use this page together with the generic materials it links to.
```

(openstack-requirements)=
## Requirements

An OpenStack version that supports:

- Compute v2 (Nova).
- Network v2 (Neutron) (optional, but required for Queens or newer).
- Volume v2 (Cinder) (optional).
- Identity v2 or v3 (Keystone).

(openstack-concepts)=
## Concepts

The following table shows how OpenStack abstractions map to Juju concepts:

| OpenStack | Juju |
| - | - |
| [Project/Tenant](https://docs.openstack.org/keystone/latest/admin/projects-users-and-roles.html) | Scope for a {ref}`model <model>` (roughly) |
| [Nova instance](https://docs.openstack.org/nova/latest/) | {ref}`machine <machine>` |
| Process on an instance | {ref}`unit <unit>` |
| Group of units for one workload | {ref}`application <application>` |
| [Cinder volume](https://docs.openstack.org/cinder/latest/) | {ref}`storage <storage>` |
| [Neutron network/subnet](https://docs.openstack.org/neutron/latest/) | Network spaces and placement targets (roughly) |

(openstack-cloud)=
## The cloud

```{ibnote}
See also: {ref}`cloud`, {ref}`Juju | Manage clouds <manage-clouds>`, {ref}`Terraform Provider for Juju | Manage clouds <tfjuju:manage-clouds>`
```

As for all machine clouds, the cloud is registered in Juju via a cloud definition, stored in `clouds.yaml` on the client (on Linux: `~/.local/share/juju/clouds.yaml`) and following this schema:

```yaml
clouds:
  <cloud-name>:  # User-defined name
    type: openstack
    auth-types:
      - <auth-type>                # See Authentication types below
    endpoint: <keystone-api-url>  # Keystone API endpoint
    regions:
      <region-name>:
        endpoint: <endpoint>       # Region-specific endpoint (if different)
    config:                        # Optional: model config defaults
      <config-key>: <value>        # See Configuration keys below
```


```{tip}
Source the OpenStack RC file (`source <path to file>`) before running `juju add-cloud` in interactive mode — Juju will detect values from preset OpenStack environment variables and suggest them as defaults.
```

(openstack-credential)=
## Credentials

```{ibnote}
See also: {ref}`credential`, {ref}`Juju | Manage credentials <manage-credentials>`, {ref}`Terraform Provider for Juju | Manage credentials <tfjuju:manage-credentials>`
```

As for all machine clouds, credentials are stored in `credentials.yaml` on the client and follow this schema:

```yaml
credentials:
  <your-openstack-cloud>  # Cloud name as defined above
    <credential-name>:             # User-defined credential name
      auth-type: <auth-type>       # userpass (see Authentication types below)
      <attribute>: <value>         # Auth-type-specific attributes (see below)
```


```{important}
**If you want to use environment variables (recommended):** Source the OpenStack RC file. Run `juju add-credential` and accept the suggested defaults.
```

(openstack-credential-authentication-types)=
### Authentication types

OpenStack supports the following authentication types:

(openstack-credential-userpass)=
#### `userpass`

Attributes:

- `username`: The username to authenticate with (required).
- `password`: The password for the specified username (required).
- `tenant-name`: The OpenStack tenant name (optional).
- `tenant-id`: The OpenStack tenant ID (optional).
- `version`: The OpenStack identity version (optional).
- `domain-name`: The OpenStack domain name (optional).
- `project-domain-name`: The OpenStack project domain name (optional).
- `user-domain-name`: The OpenStack user domain name (optional).

(openstack-controller)=
## Controllers

```{ibnote}
See also: {ref}`controller`, {ref}`Juju | Manage controllers <manage-controllers>`, {ref}`Terraform Provider for Juju | Manage controllers <tfjuju:manage-controllers>`
```

(openstack-controller-bootstrap-behavior)=
### Bootstrap behavior

Creates a controller instance on OpenStack. Requires simplestreams metadata to locate appropriate machine images. If metadata is available locally, pass it via `juju bootstrap ... --metadata-source <path to metadata simplestreams>`.

```{ibnote}
See more: {ref}`manage-metadata`
```

**Special bootstrap considerations:**

- **Multiple private networks**: Specify the network for instances to boot from via `juju bootstrap ... --model-default network=<network uuid or name>`.
- **Floating IP access**: If instances must be accessed via floating IPs, pass `allocate-public-ip=true` as a bootstrap constraint.

(openstack-controller-resources-created-at-bootstrap)=
### Resources created at bootstrap

The controller runs on a Nova instance provisioned using the same mechanisms as workload machines — see {ref}`openstack-machine-resources-created-per-machine` for the full per-machine resource model. Controller-specific differences are noted below.

**Compute**

- **Nova instance**: Ubuntu LTS compute instance. Flavor selected based on hardware constraints.
- **Instance metadata**: Tagged with `juju-is-controller: true`, `juju-controller-uuid`, and `juju-model-uuid`.

**Networking**

- **Security groups**:
  - Model-wide group: `juju-<controller-uuid>-<model-uuid>`. Ingress rules (self-referencing):
    - TCP ports 1--65535 (IPv4 and IPv6)
    - UDP ports 1--65535 (IPv4 and IPv6)
    - ICMP (IPv4 and IPv6)
  - Machine or global group (no initial rules; added via `open-ports`):
    - `firewall-mode=instance` (default): `juju-<controller-uuid>-<model-uuid>-<machine-id>`
    - `firewall-mode=global`: `juju-<controller-uuid>-<model-uuid>-global`
  - Optionally the OpenStack `default` security group if `use-default-secgroup=true`.
  - All groups tagged with `juju-controller=<controller-uuid>` and `juju-model=<model-uuid>`.
- **Network attachments**: Connected to configured internal networks from model config.
- **Neutron ports** (if space-aware networking): Pre-created with fixed IPs before instance boot.
- **Floating IP** (optional): Allocated from external network if `allocate-public-ip=true`.

**Storage**

- **Root disk**: Local ephemeral disk or Cinder boot volume based on `root-disk-source` constraint.

(openstack-model)=
## Models

```{ibnote}
See also: {ref}`model`, {ref}`Juju | Manage models <manage-models>`, {ref}`Terraform Provider for Juju | Manage models <tfjuju:manage-models>`
```

(openstack-model-configuration-keys)=
### Configuration keys

OpenStack supports the following {ref}`cloud-specific model configuration keys <model-config-cloud-specific-key>`:

**Networking**

(openstack-model-external-network)=
- **`external-network`**: The network label or UUID to create floating IP addresses on when multiple external networks exist. Type: `string`. Default: `""`.

(openstack-model-use-openstack-gbp)=
- **`use-openstack-gbp`**: Whether to use Neutron's Group-Based Policy. Type: `bool`. Default: `false`.

(openstack-model-policy-target-group)=
- **`policy-target-group`**: The UUID of Policy Target Group to use for Policy Targets created. Type: `string`. Default: `""`.

(openstack-model-use-default-secgroup)=
- **`use-default-secgroup`**: Whether new machine instances should have the "default" OpenStack security group assigned in addition to Juju-defined security groups. Type: `bool`. Default: `false`.

(openstack-model-network)=
- **`network`**: The network label or UUID to bring machines up on when multiple networks exist. Type: `string`. Default: `""`.

(openstack-machine)=
## Machines

```{ibnote}
See also: {ref}`machine`, {ref}`Juju | Manage machines <manage-machines>`, {ref}`Terraform Provider for Juju | Manage machines <tfjuju:manage-machines>`
```

(openstack-machine-constraints)=
### Constraints

OpenStack supports the following {ref}`constraints <constraint>`:

```{note}
The constraints `instance-type` and `[mem, root-disk, cores]` are mutually exclusive.
```

**Compute**

- {ref}`constraint-arch`
- {ref}`constraint-container`
- {ref}`constraint-cores`
- {ref}`constraint-image-id`. Starting with Juju 3.3. Valid values: An OpenStack image ID.
- {ref}`constraint-instance-type`. Valid values: Any user-defined OpenStack flavor.
- {ref}`constraint-mem`
- {ref}`constraint-virt-type`. Valid values: `kvm`, `lxd`.

**Networking**

- {ref}`constraint-allocate-public-ip`
- {ref}`constraint-zones`

**Storage**

- {ref}`constraint-root-disk`
- {ref}`constraint-root-disk-source`. Values: `local` (ephemeral disk, default) or `volume` (Cinder boot volume).

(openstack-machine-placement-directives)=
### Placement directives

OpenStack supports the following {ref}`placement directives <placement-directive>`:

- {ref}`placement-directive-machine`
- {ref}`placement-directive-zone`

(openstack-machine-resources-created-per-machine)=
### Resources created per machine

Applies to all machines, including controller machines. Controller-specific differences are documented in {ref}`openstack-controller-resources-created-at-bootstrap`.

**Compute**

- **Nova instance**: Compute instance with name `juju-<model-uuid>-<machine-id>`. Flavor selected based on constraints.
- **Root disk**: Local ephemeral disk (default) or Cinder boot volume if `root-disk-source=volume`.
- **Additional Cinder volumes** (optional): Created when storage specified via storage constraints.

**Networking**

- **Security groups**:
  - Model-wide group: `juju-<controller-uuid>-<model-uuid>`
  - Machine-specific group (`firewall-mode=instance`, default): `juju-<controller-uuid>-<model-uuid>-<machine-id>`
  - Global group (`firewall-mode=global`): `juju-<controller-uuid>-<model-uuid>-global`
- **Network attachments**: Connected to configured internal networks. Multiple NICs if multiple networks configured.
- **Neutron ports** (if space-aware networking): Pre-created ports with fixed IPs for each subnet/space.
- **Floating IP** (optional): Allocated from external network if `allocate-public-ip=true` constraint.

**Metadata tags:** `juju-model-uuid`, `juju-controller-uuid`, `juju-machine-id`, `juju-units-deployed`.

(openstack-machine-networking-behavior)=
### Networking behavior

- **Network selection**: Uses networks configured via `network` model config. If not specified, attaches to all available internal networks.
- **Spaces**: OpenStack supports multiple network devices. Supplying multiple space constraints or endpoint bindings will provision machines with NICs in subnets representing the union of specified spaces. Creates dedicated Neutron ports per subnet/space. Ports pre-allocated with fixed IPs before boot.
- **Security groups**: Per-model group allows internal traffic. Machine or global group allows user-defined port rules via `open-ports`.
- **Floating IPs**: Allocated from external network specified in `external-network` config. Attempts to place in same availability zone as instance. Reuses unassigned IPs when available.
- **Port security**: Respects `port_security_enabled` network attribute. Skips security group creation if port security disabled.

(openstack-machine-storage-behavior)=
### Storage behavior

```{ibnote}
See also: {ref}`storage-provider-cinder` for the Cinder storage provider configuration options.
```

- **Root disk**: Local ephemeral disk by default. Use `root-disk-source=volume` constraint to boot from a Cinder volume instead.
- **Additional volumes**: Cinder block volumes created on demand when storage is specified via storage constraints.
- **AZ constraint**: Availability zone is matched to the instance's AZ when possible.
- **Device path**: Auto-assigned by OpenStack.

(openstack-storage)=
## Storage

```{ibnote}
See also: {ref}`storage`, {ref}`Juju | Manage storage <manage-storage>`
```

(openstack-storage-providers)=
### Storage providers

In addition to generic storage providers, OpenStack provides the following {ref}`cloud-specific storage providers <storage-provider-cloud-specific>`:

(storage-provider-cinder)=
#### `cinder`

**Type:** Cinder block volumes

**Tagging:** Volumes tagged with `juju-model-uuid`, `juju-controller-uuid`, `juju-storage-instance`, `juju-storage-owner`. Volume names follow the pattern `juju-<model-uuid>-<volume-tag>`.

**Configuration options:**

- `volume-type`: The volume type. Value is the name of any volume type registered with Cinder.
