---
myst:
  html_meta:
    description: "Integrate MAAS cloud with Juju for bare metal deployments, including version requirements, authentication, and cloud credential setup."
---

(cloud-maas)=
# MAAS

In Juju, [MAAS](https://maas.io/) is a {ref}`machine cloud <machine-cloud>`. It behaves like all {ref}`machine clouds <machine-cloud>`, except for a few points of variation related to the cloud, credentials, controllers, models, machines, and storage, described below.

(maas-cloud)=
## The cloud

```{ibnote}
See also: {ref}`Juju | Manage clouds <manage-clouds>`, {ref}`Terraform Provider for Juju | Manage clouds <tfjuju:manage-clouds>`
```

(maas-cloud-definition)=
### Definition

Type in Juju: `maas`

Name in Juju: User-defined.

(maas-cloud-requirements)=
### Requirements

Starting with Juju 3.0, versions of MAAS <2 are no longer supported.

(maas-cloud-other)=
### Other

#### Resource model

MAAS differs fundamentally from public cloud providers. Instead of provisioning new infrastructure on-demand, MAAS **allocates existing machines from a pre-configured inventory**. When Juju requests a machine, MAAS finds one that matches the requirements and deploys the OS to it.

Key implications:
- All machines, networks, and storage must exist in MAAS before use.
- "Creating" a machine means allocating from inventory, not provisioning new hardware.
- Machines are released back to inventory when removed from Juju.
- Network topology must be pre-configured in MAAS (spaces, subnets, VLANs).
- Storage must exist on machine hardware -- cannot be dynamically provisioned.

(maas-credential)=
## Credentials

```{ibnote}
See also: {ref}`Juju | Manage credentials <manage-credentials>`, {ref}`Terraform Provider for Juju | Manage credentials <tfjuju:manage-credentials>`
```

(maas-credential-authentication-types)=
### Authentication types

MAAS supports the following authentication types:

(maas-credential-oauth1)=
#### `oauth1`

Attributes:
- `maas-oauth`: OAuth/API-key credentials for MAAS (required).

```{note}
`maas-oauth` is your MAAS API key. See more: [MAAS | How to add an API key for a user](https://maas.io/docs/how-to-enhance-maas-security#p-9102-manage-api-keys)
```

(maas-controller)=
## Controllers

```{ibnote}
See also: {ref}`Juju | Manage controllers <manage-controllers>`, {ref}`Terraform Provider for Juju | Manage controllers <tfjuju:manage-controllers>`
```

(maas-controller-bootstrap-behavior)=
### Bootstrap behavior

Allocates a machine from MAAS inventory that meets the specified hardware constraints. After allocation, MAAS deploys the Ubuntu OS to the machine and executes cloud-init configuration containing the controller setup.

(maas-controller-resources-created-at-bootstrap)=
### Resources created at bootstrap

MAAS does not create resources—it allocates existing machines from its inventory. The bootstrap process:

- **Machine allocation**: Requests a machine from MAAS matching hardware constraints (CPU, RAM, architecture).
- **Network interfaces**: Allocated machine must have NICs matching any space requirements from constraints.
- **Storage**: Allocated machine must have disks matching root disk size requirements.
- **Deployment**: MAAS deploys OS image and injects cloud-init userdata.
- **Tagging**: Machine tagged with `juju-is-controller: true`, `juju-controller-uuid`, and `juju-model-uuid`

All infrastructure (machines, networks, storage) must already exist in MAAS before bootstrap.

(maas-machine)=
## Machines

```{ibnote}
See also: {ref}`Juju | Manage machines <manage-machines>`, {ref}`Terraform Provider for Juju | Manage machines <tfjuju:manage-machines>`
```

(maas-machine-constraints)=
### Constraints

MAAS supports the following constraints:

- {ref}`constraint-arch`. Valid values: See cloud provider.
- {ref}`constraint-container`
- {ref}`constraint-cores`
- {ref}`constraint-image-id`. Starting with Juju 3.2. Valid values: An image name from MAAS.
- {ref}`constraint-mem`
- {ref}`constraint-root-disk`
- {ref}`constraint-spaces`
- {ref}`constraint-tags`
- {ref}`constraint-virt-type`. Starting with Juju 3.6.22. Valid values: `virtual-machine`. Default value: empty string. Use `virtual-machine` to provision a VM from a pod.
- {ref}`constraint-zones`

(maas-machine-placement-directives)=
### Placement directives

MAAS supports the following placement directives:

- {ref}`placement-directive-machine`
- {ref}`placement-directive-system-id`
- {ref}`placement-directive-zone`: If there's no '=' delimiter, assume it's a node name.

(maas-machine-resources-created-per-machine)=
### Resources created per machine

Each machine (controller or application) receives:

- **Bare metal or virtual machine**: Allocated from MAAS inventory matching hardware constraints.
- **Network interfaces**: Pre-configured NICs with IP addresses allocated from MAAS subnets.
- **Storage**: Physical disks on the machine matching storage constraints.
- **OS deployment**: Ubuntu image deployed via MAAS with Juju agent installed via cloud-init.

**Machine tags:** Machines tagged with `juju-controller-uuid`, `juju-model-uuid`, `juju-machine-id`, and `juju-units-deployed`.

(maas-machine-networking-behavior)=
### Networking behavior

- **IP addressing**: MAAS allocates IPs from configured subnet pools (static, DHCP, or auto).
- **Spaces**: Machines allocated based on required spaces from endpoint bindings and constraints.
- **Network topology**: Uses pre-existing MAAS network configuration (VLANs, subnets, spaces).
- **No provisioning**: Juju does not create networks -- all networking must be pre-configured in MAAS.

(maas-storage)=
## Storage

```{ibnote}
See also: {ref}`Juju | Manage storage <manage-storage>`
```

In addition to {ref}`generic storage providers <storage-provider>`, MAAS provides the following {ref}`cloud-specific storage providers <storage-provider-cloud-specific>`:

### Storage providers

(storage-provider-maas)=
#### `maas`

**Type:** Physical/virtual disks on MAAS machines

The MAAS storage provider is static-only—it cannot dynamically create or release volumes. Storage must exist on the machine's hardware and can only be requested at deploy time.

**Behavior:**
- Volumes are allocated from physical disks on the MAAS machine.
- Storage cannot be detached and moved to another machine.
- Volumes are removed when the machine is removed from the model.
- Cannot allocate storage to existing machines (deploy-time only).

**Limitations:** Juju cannot dissociate a MAAS disk from its machine, so attempting to deploy a unit with storage to an existing MAAS machine will return an error.

**Configuration options:**

- `tags`: A comma-separated list of tags to match on the disks in MAAS. For example, you might tag some disks as `fast`; you can then create a storage pool in Juju that will draw from the disks with those tags.

