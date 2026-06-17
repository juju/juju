---
myst:
  html_meta:
    description: "Deploy on Oracle Cloud Infrastructure (OCI) with Juju, including httpsig authentication, user OCID setup, and cloud configuration."
---

(cloud-oci)=
# Oracle OCI

In Juju, [Oracle OCI](https://docs.oracle.com/en-us/iaas/Content/home.htm) is a {ref}`machine cloud <machine-cloud>`. It behaves like all machine clouds, except for a few points of variation related to the cloud, credentials, controllers, models, machines, and storage, described below.

```{note}
This reference assumes basic familiarity with Juju. If you are new to Juju, start with the {ref}`tutorial`, then use this page together with the generic materials it links to. For a cloud-specific starting point, see {ref}`oci-appendix-example-workflows`.
```

(oci-cloud-requirements)=
## Requirements

You must specify the compartment OCID via the cloud-specific `compartment-id` model configuration key. All resources (VCNs, subnets, instances, volumes) are created in this single compartment.

Example: `juju bootstrap --config compartment-id=<compartment OCID> oracle oracle-controller`

(oci-cloud-concepts)=
## Concepts

The following table shows how OCI abstractions map to Juju concepts:

| OCI | Juju |
| - | - |
| Compute instance | {ref}`machine <machine>` |
| Process on an instance | {ref}`unit <unit>` |
| Group of units for one workload | {ref}`application <application>` |
| Block volume | {ref}`storage <storage>` |
| VCN/subnet | Network spaces and placement targets (roughly) |
| Availability domain | Placement target (`zones`) |

(oci-cloud)=
## The cloud

```{ibnote}
See also: {ref}`cloud`, {ref}`Juju | Manage clouds <manage-clouds>`, {ref}`Terraform Provider for Juju | Manage clouds <tfjuju:manage-clouds>`
```

Type in Juju: `oci`

Name in Juju: `oracle` (predefined)

(oci-credential)=
## Credentials

```{ibnote}
See also: {ref}`credential`, {ref}`Juju | Manage credentials <manage-credentials>`, {ref}`Terraform Provider for Juju | Manage credentials <tfjuju:manage-credentials>`
```

(oci-credential-authentication-types)=
### Authentication types

Oracle OCI supports the following authentication types:

(oci-credential-httpsig)=
#### `httpsig`

Attributes:

- `user`: Username OCID (required).
- `tenancy`: Tenancy OCID (required).
- `key`: PEM encoded private key (required).
- `pass-phrase`: Passphrase used to unlock the key (required).
- `fingerprint`: Private key fingerprint (required).
- `region`: DEPRECATED -- Region to log into (required).

(oci-controller)=
## Controllers

```{ibnote}
See also: {ref}`controller`, {ref}`Juju | Manage controllers <manage-controllers>`, {ref}`Terraform Provider for Juju | Manage controllers <tfjuju:manage-controllers>`
```

(oci-controller-bootstrap-behavior)=
### Bootstrap behavior

Creates a controller instance on OCI by provisioning the required network and compute resources, then waiting for them to become ready.

(oci-controller-resources-created-at-bootstrap)=
### Resources created at bootstrap

- **Virtual Cloud Network (VCN)**: CIDR block from `address-space` config (default: `10.0.0.0/16`). Name: `juju-vcn-<controller-uuid>-<model-uuid>`.
- **Security list**: Permissive by default -- allows all ingress/egress (`0.0.0.0/0`, all protocols). Name: `juju-seclist-<controller-uuid>-<model-uuid>`. Applied at subnet level.
- **Internet gateway**: Enables public internet routing for the VCN.
- **Route table**: Default route `0.0.0.0/0` to Internet Gateway. Name: `juju-rt-<controller-uuid>-<model-uuid>`.
- **Subnets**: One per availability domain. CIDR `/24` auto-selected from VCN address space. Name: `juju-<availability-domain>-<controller-uuid>-<model-uuid>`.
- **Availability-domain layout**: Bootstrap discovers region availability domains and prepares network resources for each one.
- **Controller instance**: Boot volume (minimum 50 GiB), VNIC with optional public IP, and instance type from constraints (default flexible shape).
- **Freeform tags**: All resources tagged with `JujuController=<controller-uuid>`, `JujuModel=<model-uuid>`. Controller instances also tagged `JujuIsController=true`.

(oci-model)=
## Models

```{ibnote}
See also: {ref}`model`, {ref}`Juju | Manage models <manage-models>`, {ref}`Terraform Provider for Juju | Manage models <tfjuju:manage-models>`
```

(oci-model-configuration-keys)=
### Configuration keys

Oracle OCI supports the following {ref}`cloud-specific model configuration keys <model-config-cloud-specific-key>`:

(oci-model-compartment-id)=
#### `compartment-id`

The OCID of the compartment in which Juju has access to create resources.

- **Type**: `string`
- **Default value**: `""`
- **Immutable**: `false`
- **Mandatory**: `false`

(oci-model-address-space)=
#### `address-space`

The CIDR block to use when creating default subnets. The subnet must have at least a `/16` size.

- **Type**: `string`
- **Default value**: `"10.0.0.0/16"`
- **Immutable**: `false`
- **Mandatory**: `false`

(oci-machine)=
## Machines

```{ibnote}
See also: {ref}`machine`, {ref}`Juju | Manage machines <manage-machines>`, {ref}`Terraform Provider for Juju | Manage machines <tfjuju:manage-machines>`
```

(oci-machine-constraints)=
### Constraints

Oracle OCI supports the following {ref}`constraints <constraint>`:

- {ref}`constraint-allocate-public-ip`
- {ref}`constraint-arch`. Valid values: `amd64`, `arm64`.
- {ref}`constraint-cores`
- {ref}`constraint-cpu-power`
- {ref}`constraint-instance-type`. Valid values: Any OCI shape. Examples: `VM.Standard.E4.Flex` (flexible VM), `BM.Standard.E4.Bare` (bare metal), `VM.Standard.A1.Flex` (Ampere ARM), `BM.GPU.A100-v2` (GPU).
- {ref}`constraint-mem`
- {ref}`constraint-root-disk`
- {ref}`constraint-zones`. Specifies availability domain. Example: `zones=us-phoenix-1:AD-1`.

(oci-machine-placement-directives)=
### Placement directives

Oracle OCI supports the following placement directives:

- {ref}`placement-directive-machine`
- {ref}`placement-directive-zone`

(oci-machine-resources-created-per-machine)=
### Resources created per machine

Each machine (controller or application) receives:

- **Compute instance**: Shape from constraint (default flexible shape). Image auto-selected by OS and architecture.
- **Availability-domain selection**: Without `zones` constraints, Juju launches machines in the first available AD. With `zones`, Juju targets the specified AD.
- **Boot volume**: Created during instance launch. Size: minimum 50 GiB, maximum 16 TiB. From `root-disk` constraint or default 50 GiB. Lifecycle tied to instance.
- **VNIC**: Created during instance launch. Subnet: first subnet of target availability domain. Private IP auto-assigned. Public IP optional (default enabled).
- **Flexible shape configuration** (if applicable): For flexible shapes (e.g., `VM.Standard.A1.Flex`), OCPUs and memory are set from constraints or defaults.
- **Instance metadata**: Bootstrap metadata is written for instance initialization. VMs can query the OCI metadata service at `169.254.169.254`.
- **Freeform tags**: `JujuController=<controller-uuid>`, `JujuModel=<model-uuid>`. User-provided tags from instance config.
- **Additional block volumes** (optional): Created when storage is specified. Attached over iSCSI with CHAP enabled. Must be in same availability domain as the instance.

(oci-machine-networking-behavior)=
### Networking behavior

- **VCN architecture**: One VCN per model. All machines in model share VCN.
- **Subnet selection**: One subnet per availability domain. Instance uses first subnet of its target AD.
- **IP address management**: Private IPs obtained via `Networking.GetVnic()` after VNIC attachment. Public IPs optional, queried from same VNIC. Private scope: `ScopeCloudLocal`. Public scope: `ScopePublic`.
- **Security model**: Network-level security list (all ports open by default) applied at subnet level. Instance-level firewall via SSH -- `open-ports`/`close-ports` translate to SSH rule modifications. Limitation: Cannot specify target prefix per rule.
- **Routing**: All subnets route `0.0.0.0/0` through Internet Gateway. No custom routes currently managed.
- **Public IP allocation**: Not guaranteed immediately. Juju polls up to 30 seconds after instance reaches Running state.

(oci-storage)=
## Storage

```{ibnote}
See also: {ref}`storage`, {ref}`Juju | Manage storage <manage-storage>`
```

In addition to generic storage providers, Oracle OCI provides the following {ref}`cloud-specific storage providers <storage-provider-cloud-specific>`:

### Storage providers

(storage-provider-oracle)=
#### `oracle`

**Type:** OCI block volumes (iSCSI)

**Configuration options:**

- `volume-type`: The volume type. Valid values: `default` (associated with Juju pool `oracle`) or `latency` (associated with Juju pool `oracle-latency`). Use `latency` for low-latency, high IOPS requirements, and `default` otherwise.

**Behavior:**

- Volumes are created on demand. Size: 50-16,000 GiB.
- Attached via iSCSI with CHAP enabled.
- Must be in same availability domain as target instance.
- Juju waits for volume and attachment readiness before declaring storage available.

(oci-appendix-example-workflows)=
## Appendix: Example workflows

(oci-appendix-quickstart)=
### Add cloud, add credential, bootstrap


1. Add or confirm the predefined cloud with `juju add-cloud`.
2. Add credentials with `juju add-credential oracle` and choose `httpsig`.
3. Bootstrap with `juju bootstrap --config compartment-id=<compartment-ocid> oracle oci-controller`.