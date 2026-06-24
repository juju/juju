---
myst:
  html_meta:
    description: "Integrate MAAS cloud with Juju for bare metal deployments, including version requirements, authentication, and cloud credential setup."
---

(cloud-maas)=
# MAAS

In Juju, [MAAS](https://maas.io/) is a {ref}`machine cloud <machine-cloud>` and works as described below.

```{note}
This reference assumes basic familiarity with Juju. If you are new to Juju, start with the {ref}`Tutorial <tutorial>`, then use this page together with the generic materials it links to and/or consult the {ref}`example workflows <maas-appendix-example-workflows>`.
```

(maas-limitations)=
## Limitations

- **Pre-existing infrastructure required**: All machines, networks, and storage must exist in MAAS before use. Juju does not provision new hardware.
- **Spaces inherited from MAAS**: Juju reads MAAS spaces and subnets but does not create or modify them. If spaces or subnets change in MAAS, reload them with `juju reload-spaces`. Note: the `alpha` space does not exist in MAAS, so applications are not connected to any space by default. Use `default-space` in model config or bind applications explicitly to an existing MAAS space.
- **Static storage only**: The MAAS storage provider cannot dynamically create or release volumes. Storage must exist on machine hardware and can only be requested at deploy time. Juju cannot dissociate a MAAS disk from its machine -- attempting to deploy a unit with storage to an existing MAAS machine returns an error.
- **Machines released on removal**: When a machine is removed from a Juju model, it is released back to the MAAS inventory rather than destroyed.

(maas-requirements)=
## Requirements

Starting with Juju 3.0, MAAS versions earlier than 2 are no longer supported.

(maas-concepts)=
## Concepts

The following table shows how MAAS abstractions map to Juju concepts:

| MAAS | Juju |
| - | - |
| Allocated machine from inventory | {ref}`machine <machine>` |
| Process on a commissioned machine | {ref}`unit <unit>` |
| Group of units for one workload | {ref}`application <application>` |
| MAAS-managed disks and filesystems | {ref}`storage <storage>` |
| MAAS spaces/fabrics/subnets | Network spaces and placement targets |
| MAAS API key (`maas-oauth`) | Cloud credential |

(maas-cloud)=
## The cloud

```{ibnote}
See also: {ref}`cloud`, {ref}`Juju | Manage clouds <manage-clouds>`, {ref}`Terraform Provider for Juju | Manage clouds <tfjuju:manage-clouds>`
```

As for all machine clouds, the cloud is registered in Juju via a cloud definition, stored in `clouds.yaml` on the client (on Linux: `~/.local/share/juju/clouds.yaml`) and following this schema:

```yaml
clouds:
  <cloud-name>:  # User-defined name
    type: maas
    auth-types:
      - <auth-type>                # See Authentication types below
    endpoint: <maas-api-url>       # MAAS API endpoint
    config:                        # Optional: model config defaults
      <config-key>: <value>        # See Configuration keys below
```


(maas-credential)=
## Credentials

```{ibnote}
See also: {ref}`credential`, {ref}`Juju | Manage credentials <manage-credentials>`, {ref}`Terraform Provider for Juju | Manage credentials <tfjuju:manage-credentials>`
```

As for all machine clouds, credentials are stored in `credentials.yaml` on the client and follow this schema:

```yaml
credentials:
  <your-maas-cloud>           # Cloud name as defined above
    <credential-name>:             # User-defined credential name
      auth-type: <auth-type>       # oauth1 (the only type)
      <attribute>: <value>         # Auth-type-specific attributes (see below)
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
See also: {ref}`controller`, {ref}`Juju | Manage controllers <manage-controllers>`, {ref}`Terraform Provider for Juju | Manage controllers <tfjuju:manage-controllers>`
```

(maas-controller-bootstrap-behavior)=
### Bootstrap behavior

Allocates a machine from MAAS inventory that meets the specified hardware constraints. After allocation, MAAS deploys the Ubuntu OS to the machine and executes cloud-init configuration containing the controller setup.

(maas-controller-resources-created-at-bootstrap)=
### Resources allocated at bootstrap

MAAS allocates (rather than creates) existing machines from inventory. The controller runs on a machine provisioned using the same mechanisms as workload machines — see {ref}`maas-machine-resources-created-per-machine` for the full per-machine resource model. Controller-specific differences are noted below.

**Compute**

- **Machine allocation**: A machine from MAAS inventory matching hardware constraints (CPU, RAM, architecture) is allocated and commissioned.

**Networking**

- **Network interfaces**: Allocated machine must have NICs matching any space requirements from constraints.

**Storage**

- **Disks**: Allocated machine must have disks matching root disk size requirements.

**Deployment**: MAAS deploys the OS image and injects cloud-init userdata. The machine is tagged with `juju-is-controller: true`, `juju-controller-uuid`, and `juju-model-uuid`.

(maas-machine)=
## Machines

```{ibnote}
See also: {ref}`machine`, {ref}`Juju | Manage machines <manage-machines>`, {ref}`Terraform Provider for Juju | Manage machines <tfjuju:manage-machines>`
```

MAAS provides two modes of machine provisioning, selected via the `virt-type` constraint:

- **Default (bare metal or pre-existing VM)**: Juju calls `AllocateMachine` to allocate an existing machine from the MAAS inventory. Nothing is created.
- **`virt-type=virtual-machine`**: Juju calls `ComposeMachine` to create a new VM from a MAAS pod (KVM or LXD host). The pod must pre-exist in MAAS, but the VM itself is created on demand.

(maas-machine-constraints)=
### Constraints

MAAS supports the following {ref}`constraints <constraint>`:

**Compute**

- {ref}`constraint-arch`. Valid values: See cloud provider.
- {ref}`constraint-cores`
- {ref}`constraint-image-id`. Starting with Juju 3.2. Valid values: An image name from MAAS.
- {ref}`constraint-mem`
- {ref}`constraint-virt-type`. Starting with Juju 3.6.22. Valid values: `virtual-machine`. Default: empty string (allocates from inventory). Use `virtual-machine` to compose a VM from a pod.

**Networking**

- {ref}`constraint-spaces`
- {ref}`constraint-zones`

**Storage**

- {ref}`constraint-root-disk`

**Other**

- {ref}`constraint-container`
- {ref}`constraint-tags`. Used to match machines with specific MAAS tags.

(maas-machine-placement-directives)=
### Placement directives

MAAS supports the following {ref}`placement directives <placement-directive>`:

- {ref}`placement-directive-machine`
- {ref}`placement-directive-system-id`
- {ref}`placement-directive-zone`: If there's no '=' delimiter, assume it's a node name.

(maas-machine-resources-created-per-machine)=
### Resources allocated/created per machine

Applies to all machines, including controller machines. Controller-specific differences are documented in {ref}`maas-controller-resources-created-at-bootstrap`.

**Compute**

- **Machine**: Allocated from MAAS inventory (bare metal or pre-existing VM) matching hardware constraints, or composed as a new VM from a pod when `virt-type=virtual-machine`.

**Networking**

- **Network interfaces**: Pre-configured NICs with IP addresses allocated from MAAS subnets.

**Storage**

- **Disks**: Physical disks on the machine matching storage constraints.

**Deployment**: Ubuntu image deployed via MAAS with Juju agent installed via cloud-init. Machine tagged with `juju-controller-uuid`, `juju-model-uuid`, `juju-machine-id`, and `juju-units-deployed`.

(maas-machine-networking-behavior)=
### Networking behavior

- **IP addressing**: MAAS allocates IPs from configured subnet pools (static, DHCP, or auto).
- **Spaces**: Juju reads MAAS spaces and subnets but does not create or modify them. Machines are allocated based on required spaces from endpoint bindings and constraints.
- **Network topology**: Uses pre-existing MAAS network configuration (VLANs, subnets, spaces). Juju does not provision networks.

(maas-machine-storage-behavior)=
### Storage behavior

```{ibnote}
See also: {ref}`storage-provider-maas` for the MAAS storage provider configuration options.
```

- **Physical disks only**: Storage must exist on machine hardware. Juju cannot dynamically provision storage volumes.
- **Deploy-time only**: Storage can only be requested at deploy time; it cannot be added to existing machines.
- **No detachment**: Juju cannot dissociate a MAAS disk from its machine.
- **Released on removal**: Storage is removed when the machine is removed from the model.

(maas-storage)=
## Storage

```{ibnote}
See also: {ref}`storage`, {ref}`Juju | Manage storage <manage-storage>`
```

(maas-storage-providers)=
### Storage providers

In addition to generic storage providers, MAAS provides the following {ref}`cloud-specific storage providers <storage-provider-cloud-specific>`:

(storage-provider-maas)=
#### `maas`

**Type:** Physical/virtual disks on MAAS machines

**Configuration options:**

- `tags`: A comma-separated list of tags to match on the disks in MAAS. For example, tag some disks as `fast` and create a Juju storage pool that draws from disks with that tag.

(maas-appendix-example-workflows)=
## Appendix: Example workflows

(maas-appendix-quickstart)=
### Add cloud, add credential, bootstrap

1. Add the MAAS cloud endpoint with `juju add-cloud`.
2. Add credentials with `juju add-credential` and choose `oauth1`.
3. Bootstrap with `juju bootstrap <maas-cloud-name> maas-controller`.
