---
myst:
  html_meta:
    description: "Deploy on Oracle Cloud Infrastructure (OCI) with Juju, including httpsig authentication, user OCID setup, and cloud configuration."
---

(cloud-oci)=
# Oracle OCI

In Juju, [Oracle OCI](https://docs.oracle.com/en-us/iaas/Content/home.htm) is a {ref}`machine cloud <machine-cloud>` and works as described below.

```{note}
This reference assumes basic familiarity with Juju. If you are new to Juju, start with the {ref}`Tutorial <tutorial>`, then use this page together with the generic materials it links to.
```

(oci-requirements)=
## Requirements

You must specify the compartment OCID via the `compartment-id` model configuration key before bootstrapping. All resources (VCNs, subnets, instances, volumes) are created in this single compartment.

Example: `juju bootstrap --config compartment-id=<compartment OCID> oracle oracle-controller`

(oci-concepts)=
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

As for all machine clouds, the cloud is registered in Juju via a cloud definition, stored in `clouds.yaml` on the client (on Linux: `~/.local/share/juju/clouds.yaml`) and following this schema:

```yaml
clouds:
  <cloud-name>:  # Predefined name
    type: oci
    auth-types:
      - <auth-type>                # See Authentication types below
    regions:
      <region-name>:               # e.g. us-phoenix-1
        endpoint: <endpoint>       # Region-specific OCI API endpoint
    config:                        # Optional: model config defaults
      <config-key>: <value>        # See Configuration keys below
```


(oci-credential)=
## Credentials

```{ibnote}
See also: {ref}`credential`, {ref}`Juju | Manage credentials <manage-credentials>`, {ref}`Terraform Provider for Juju | Manage credentials <tfjuju:manage-credentials>`
```

As for all machine clouds, credentials are stored in `credentials.yaml` on the client and follow this schema:

```yaml
credentials:
  oracle                         # Predefined cloud name for OCI
    <credential-name>:             # User-defined credential name
      auth-type: <auth-type>       # httpsig (the only type)
      <attribute>: <value>         # Auth-type-specific attributes (see below)
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

The controller runs on an OCI instance provisioned using the same mechanisms as workload machines — see {ref}`oci-machine-resources-created-per-machine` for the full per-machine resource model. Controller-specific differences are noted below.

**Compute**

- **Controller instance**: Boot volume (minimum 50 GiB), VNIC with optional public IP, and instance type from constraints (default flexible shape).
- **Freeform tags**: All resources tagged with `JujuController=<controller-uuid>`, `JujuModel=<model-uuid>`. Controller instances also tagged `JujuIsController=true`.

**Networking**

- **Virtual Cloud Network (VCN)**: CIDR block from `address-space` config (default: `10.0.0.0/16`). Name: `juju-vcn-<controller-uuid>-<model-uuid>`.
- **Security list**: Permissive by default -- allows all ingress/egress (`0.0.0.0/0`, all protocols). Name: `juju-seclist-<controller-uuid>-<model-uuid>`. Applied at subnet level.
- **Internet gateway**: Enables public internet routing for the VCN.
- **Route table**: Default route `0.0.0.0/0` to Internet Gateway. Name: `juju-rt-<controller-uuid>-<model-uuid>`.
- **Subnets**: One per availability domain. CIDR `/24` auto-selected from VCN address space. Name: `juju-<availability-domain>-<controller-uuid>-<model-uuid>`.
- **Availability-domain layout**: Bootstrap discovers region availability domains and prepares network resources for each one.

(oci-model)=
## Models

```{ibnote}
See also: {ref}`model`, {ref}`Juju | Manage models <manage-models>`, {ref}`Terraform Provider for Juju | Manage models <tfjuju:manage-models>`
```

(oci-model-configuration-keys)=
### Configuration keys

Oracle OCI supports the following {ref}`cloud-specific model configuration keys <model-config-cloud-specific-key>`:

**Compute**

(oci-model-compartment-id)=
- **`compartment-id`**: The OCID of the compartment in which Juju has access to create resources. Type: `string`. Default: `""`.

**Networking**

(oci-model-address-space)=
- **`address-space`**: The CIDR block to use when creating default subnets. The subnet must have at least a `/16` size. Type: `string`. Default: `"10.0.0.0/16"`.

(oci-machine)=
## Machines

```{ibnote}
See also: {ref}`machine`, {ref}`Juju | Manage machines <manage-machines>`, {ref}`Terraform Provider for Juju | Manage machines <tfjuju:manage-machines>`
```

(oci-machine-constraints)=
### Constraints

Oracle OCI supports the following {ref}`constraints <constraint>`:

**Compute**

- {ref}`constraint-arch`. Valid values: `amd64`, `arm64`.
- {ref}`constraint-cores`
- {ref}`constraint-cpu-power`
- {ref}`constraint-instance-type`. Valid values: Any OCI shape. Examples: `VM.Standard.E4.Flex` (flexible VM), `BM.Standard.E4.Bare` (bare metal), `VM.Standard.A1.Flex` (Ampere ARM), `BM.GPU.A100-v2` (GPU).
- {ref}`constraint-mem`

**Networking**

- {ref}`constraint-allocate-public-ip`
- {ref}`constraint-zones`. Specifies availability domain. Example: `zones=us-phoenix-1:AD-1`.

**Storage**

- {ref}`constraint-root-disk`

(oci-machine-placement-directives)=
### Placement directives

Oracle OCI supports the following {ref}`placement directives <placement-directive>`:

- {ref}`placement-directive-machine`
- {ref}`placement-directive-zone`

(oci-machine-resources-created-per-machine)=
### Resources created per machine

Applies to all machines, including controller machines. Controller-specific defaults are documented in {ref}`oci-controller-resources-created-at-bootstrap`.

**Compute**

- **Compute instance**: Shape from constraint (default flexible shape). Image auto-selected by OS and architecture.
- **Availability-domain selection**: Without `zones` constraints, Juju launches machines in the first available AD. With `zones`, Juju targets the specified AD.
- **Flexible shape configuration** (if applicable): For flexible shapes (e.g., `VM.Standard.A1.Flex`), OCPUs and memory are set from constraints or defaults.
- **Instance metadata**: Bootstrap metadata is written for instance initialization. VMs can query the OCI metadata service at `169.254.169.254`.
- **Freeform tags**: `JujuController=<controller-uuid>`, `JujuModel=<model-uuid>`. User-provided tags from instance config.

**Networking**

- **VNIC**: Created during instance launch. Subnet: first subnet of target availability domain. Private IP auto-assigned. Public IP optional (default enabled).

**Storage**

- **Boot volume**: Created during instance launch. Size: minimum 50 GiB, maximum 16 TiB. From `root-disk` constraint or default 50 GiB. Lifecycle tied to instance.
- **Additional block volumes** (optional): Created when storage is specified. Attached over iSCSI with CHAP enabled. Must be in same availability domain as the instance.

(oci-machine-networking-behavior)=
### Networking behavior

- **VCN architecture**: One VCN per model. All machines in model share VCN.
- **Subnet selection**: One subnet per availability domain. Instance uses first subnet of its target AD.
- **IP address management**: Private IPs obtained via `Networking.GetVnic()` after VNIC attachment. Public IPs optional, queried from same VNIC. Private scope: `ScopeCloudLocal`. Public scope: `ScopePublic`.
- **Security model**: Network-level security list (all ports open by default) applied at subnet level. Instance-level firewall via SSH -- `open-ports`/`close-ports` translate to SSH rule modifications. Limitation: Cannot specify target prefix per rule.
- **Routing**: All subnets route `0.0.0.0/0` through Internet Gateway. No custom routes currently managed.
- **Public IP allocation**: Not guaranteed immediately. Juju polls up to 30 seconds after instance reaches Running state.

(oci-machine-storage-behavior)=
### Storage behavior

```{ibnote}
See also: {ref}`storage-provider-oracle` for the OCI storage provider configuration options.
```

- **Boot volume**: Minimum 50 GiB, maximum 16 TiB. Lifecycle tied to instance.
- **Additional volumes**: Attached via iSCSI with CHAP enabled. Must be in the same availability domain as the instance. Juju waits for volume and attachment readiness before declaring storage available.

(oci-storage)=
## Storage

```{ibnote}
See also: {ref}`storage`, {ref}`Juju | Manage storage <manage-storage>`
```

(oci-storage-providers)=
### Storage providers

In addition to generic storage providers, Oracle OCI provides the following {ref}`cloud-specific storage providers <storage-provider-cloud-specific>`:

(storage-provider-oracle)=
#### `oracle`

**Type:** OCI block volumes (iSCSI)

**Configuration options:**

- `volume-type`: The volume type. Valid values: `default` (associated with Juju pool `oracle`) or `latency` (associated with Juju pool `oracle-latency`). Use `latency` for low-latency, high IOPS requirements, and `default` otherwise.
