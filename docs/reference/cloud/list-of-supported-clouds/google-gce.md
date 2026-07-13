---
myst:
  html_meta:
    description: "Configure Google Compute Engine (GCE) cloud with Juju, including IAM permissions, service account setup, and authentication types."
---

(cloud-gce)=
# Google GCE

In Juju, [Google GCE](https://cloud.google.com/compute/docs) is a {ref}`machine cloud <machine-cloud>` and works as described below.

```{note}
This reference assumes basic familiarity with Juju. If you are new to Juju, start with the {ref}`Tutorial <tutorial>`, then use this page together with the generic materials it links to and/or consult the {ref}`example workflows <gce-appendix-example-workflows>`.
```

(gce-requirements)=
## Requirements

Juju needs Service Account Key Admin, Compute Instance Admin, and Compute Security Admin to create and manage the GCE resources used during cloud registration and bootstrap.

```{ibnote}
See more: [Google | Compute Engine IAM roles and permissions](https://cloud.google.com/compute/docs/access/iam)
```

(gce-cloud-concepts)=
## Concepts

The following table shows how GCE abstractions map to Juju concepts:

| GCE | Juju |
| - | - |
| [Project](https://cloud.google.com/resource-manager/docs/creating-managing-projects) | Administrative boundary for {ref}`models <model>` (roughly) |
| [Compute Engine instance](https://cloud.google.com/compute/docs/instances) | {ref}`machine <machine>` |
| Process on a VM | {ref}`unit <unit>` |
| Managed set of workload instances | {ref}`application <application>` |
| [Persistent Disk](https://cloud.google.com/compute/docs/disks) | {ref}`storage <storage>` |
| [VPC/subnet](https://cloud.google.com/vpc/docs) | Network spaces and placement targets (roughly) |

(gce-cloud)=
## The cloud

```{ibnote}
See also: {ref}`cloud`, {ref}`Juju | Manage clouds <manage-clouds>`, {ref}`Terraform Provider for Juju | Manage clouds <tfjuju:manage-clouds>`
```

As for all machine clouds, the cloud is registered in Juju via a cloud definition, stored in `clouds.yaml` on the client (on Linux: `~/.local/share/juju/clouds.yaml`) and following this schema:

```yaml
clouds:
  <cloud-name>:  # Predefined name
    type: gce
    auth-types:
      - <auth-type>                # See Authentication types below
    regions:
      <region-name>:               # e.g. us-central1
        endpoint: <endpoint>       # Region-specific GCE API endpoint
    config:                        # Optional: model config defaults
      <config-key>: <value>        # See Configuration keys below
```


(gce-credential)=
## Credentials

```{ibnote}
See also: {ref}`credential`, {ref}`Juju | Manage credentials <manage-credentials>`, {ref}`Terraform Provider for Juju | Manage credentials <tfjuju:manage-credentials>`
```

As for all machine clouds, credentials are stored in `credentials.yaml` on the client and follow this schema:

```yaml
credentials:
  google                         # Predefined cloud name for GCE
    <credential-name>:             # User-defined credential name
      auth-type: <auth-type>       # oauth2 | jsonfile | service-account (see Authentication types below)
      <attribute>: <value>         # Auth-type-specific attributes (see below)
```


(gce-credential-authentication-types)=
### Authentication types

Google GCE supports the following authentication types:

(gce-credential-oauth2)=
#### `oauth2`

Attributes:

- `client-id`: Client ID (required).
- `client-email`: Client e-mail address (required).
- `private-key`: Client secret (required).
- `project-id`: Project ID (required).

(gce-credential-jsonfile)=
#### `jsonfile`

Attributes:

- `file`: Path to the `.json` file containing a service account key for your project (required).

**Auto-detection:** If `GOOGLE_APPLICATION_CREDENTIALS` is set to a valid file path, `juju autoload-credentials` detects this credential type automatically. If `CLOUDSDK_COMPUTE_REGION` is also set, it becomes the default region for the detected credential.

```{ibnote}
See more: {ref}`gce-appendix-workflow-2`
```

(gce-credential-service-account)=
#### `service-account`

**Requirements:**

- Juju 3.6+
- A service account with sufficient privileges:
  - `https://www.googleapis.com/auth/compute`
  - `https://www.googleapis.com/auth/devstorage.full_control`
- The `add-credential` steps must be run from a jump host running in Google Cloud to reach the cloud metadata endpoint.

```{ibnote}
See more: {ref}`gce-appendix-workflow-1`
```

(gce-controller)=
## Controllers

```{ibnote}
See also: {ref}`controller`, {ref}`Juju | Manage controllers <manage-controllers>`, {ref}`Terraform Provider for Juju | Manage controllers <tfjuju:manage-controllers>`
```

(gce-controller-bootstrap-behavior)=
### Bootstrap behavior

Creates a controller instance on GCE in a single API request. Juju creates the required GCE resources directly -- no templates.

(gce-controller-resources-created-at-bootstrap)=
### Resources created at bootstrap

The controller runs on a GCE instance provisioned using the same mechanisms as workload machines -- see {ref}`gce-machine-resources-created-per-machine` for the full per-machine resource model. Controller-specific differences are noted below.

**Compute**

- **Compute instance**: Ubuntu LTS instance. Machine type selected based on hardware constraints (default `n1-standard-1`). Instance creation includes boot disk inline.
- **Service account** (optional): Attached if credential type is `service-account` or `instance-role` constraint specified. Scopes: `compute`, `devstorage.full_control`. Enables metadata service credentials.
- **Instance metadata**: Tagged with `juju-controller-uuid`, `juju-is-controller: true`, and bootstrap metadata.
- **Instance tags**: `juju-<model-uuid>` (for firewall targeting), hostname.

**Networking**

- **Network interface**: Primary interface in specified VPC/subnet or default network. Private IP auto-assigned from subnet CIDR. External NAT with public IP if `allocate-public-ip=true` (default).
- **Firewall rule**: Global VPC firewall rule `juju-<model-uuid>` targeting instances tagged `juju-<model-uuid>`. Created with no initial rules; rules are added dynamically via `open-ports`.

**Storage**

- **Boot disk**: Persistent disk, device name auto-assigned. Default 10 GiB minimum (expanded if constraint/image requires). Type `pd-standard` (default) or `pd-ssd`. Auto-deleted when instance terminates.

(gce-model)=
## Models

```{ibnote}
See also: {ref}`model`, {ref}`Juju | Manage models <manage-models>`, {ref}`Terraform Provider for Juju | Manage models <tfjuju:manage-models>`
```

(gce-model-configuration-keys)=
### Configuration keys

Google GCE supports the following {ref}`cloud-specific model configuration keys <model-config-cloud-specific-key>`:

**Networking**

(gce-model-vpc-id)=
- **`vpc-id`**: Use a specific VPC network. When not specified, Juju requires a default VPC to be available for the account. Example: `vpc-a1b2c3d4`. Type: `string`. Default: `""`. Immutable.

(gce-model-vpc-id-force)=
- **`vpc-id-force`**: Force Juju to use the GCE VPC ID specified with `vpc-id`, when it fails the minimum validation criteria. Type: `bool`. Default: `false`. Immutable.

**Storage**

(gce-model-base-image-path)=
- **`base-image-path`**: Base path to look for machine disk images. Type: `string`. Default: none.

(gce-machine)=
## Machines

```{ibnote}
See also: {ref}`machine`, {ref}`Juju | Manage machines <manage-machines>`, {ref}`Terraform Provider for Juju | Manage machines <tfjuju:manage-machines>`
```

(gce-machine-constraints)=
### Constraints

Google GCE supports the following {ref}`constraints <constraint>`:

```{note}
The constraints `instance-type` and `[cores, cpu-power, mem]` are mutually exclusive.
```

**Compute**

- {ref}`constraint-arch`
- {ref}`constraint-container`
- {ref}`constraint-cores`
- {ref}`constraint-cpu-power`
- {ref}`constraint-instance-role`. Valid values: A service account email.
- {ref}`constraint-instance-type`. Valid values: Any GCE machine type. Default: `n1-standard-1`.
- {ref}`constraint-mem`

**Networking**

- {ref}`constraint-allocate-public-ip`
- {ref}`constraint-spaces`
- {ref}`constraint-zones`

**Storage**

- {ref}`constraint-root-disk`
- {ref}`constraint-root-disk-source`

(gce-machine-placement-directives)=
### Placement directives

Google GCE supports the following {ref}`placement directives <placement-directive>`:

- {ref}`placement-directive-machine`
- {ref}`placement-directive-subnet`: Matches subnet by name or CIDR range.
- {ref}`placement-directive-zone`

(gce-machine-resources-created-per-machine)=
### Resources created per machine

Applies to all machines, including controller machines. Controller-specific defaults are documented in {ref}`gce-controller-resources-created-at-bootstrap`.

**Compute**

- **Compute instance**: Instance with name `<model-uuid><machine-id>`. Machine type selected based on constraints. Status sequence: `PROVISIONING` → `STAGING` → `RUNNING`.
- **Service account** (optional): Attached if `instance-role` constraint specified. Enables metadata service credentials.
- **Instance metadata**: Bootstrap metadata, controller UUID, and model UUID.
- **Instance tags**: `juju-<model-uuid>`, hostname (for firewall targeting).

**Networking**

- **Network interface**: Primary interface in VPC/subnet. Private IP auto-assigned. External NAT with public IP if `allocate-public-ip=true`.

**Storage**

- **Boot disk**: Persistent disk attached inline. Size: max(10 GiB, constraint, image minimum). Type: `pd-standard` (default) or `pd-ssd` via `root-disk-source` constraint. Auto-deleted when instance terminates.
- **Additional persistent disks** (optional): Created when storage specified via storage constraints. Must reside in same zone as instance.
- **Disk labels**: `juju-model`, `juju-controller` (set via upgrade step).

(gce-machine-networking-behavior)=
### Networking behavior

- **VPC requirements**: If you use a VPC, Juju validates the configuration before bootstrap. A valid VPC must have: at least one subnet with status `READY`, OR `AutoCreateSubnetworks=true` enabled; SSH access enabled (firewall rule for port 22).
- **VPC/subnet selection**: Uses VPC configured via `vpc-id` model config or default network (`global/networks/default`). Subnet selection driven by `zone` or `subnet` placement directives. Random selection from available subnets in region. Space constraints filter to valid subnets.
- **Public IP handling**: Assigned via external NAT (`ONE_TO_ONE_NAT`) if `allocate-public-ip=true` (default). Ephemeral public IP auto-assigned by GCE.
- **Firewall rules**: Environment-level rule (`juju-<model-uuid>`) allows traffic between instances with same tag. Per-machine rules target instance by hostname tag. User-defined port rules via `open-ports` create additional firewall rules.
- **Address resolution**: Returns private address (cloud-local scope, from subnet CIDR) and public address (if NAT configured).

(gce-machine-storage-behavior)=
### Storage behavior

```{ibnote}
See also: {ref}`storage-provider-gce` for the GCE storage provider configuration options.
```

- **Boot disk**: Persistent disk, type `pd-standard` by default. Configurable via `root-disk-source` constraint (specify a storage pool with `disk-type`).
- **Additional disks**: Must reside in the same availability zone as the instance.
- **Auto-deletion**: Boot disks are auto-deleted when the instance terminates.

(gce-storage)=
## Storage

```{ibnote}
See also: {ref}`storage`, {ref}`Juju | Manage storage <manage-storage>`
```

(gce-storage-providers)=
### Storage providers

In addition to generic storage providers, Google GCE provides the following {ref}`cloud-specific storage providers <storage-provider-cloud-specific>`:

(storage-provider-gce)=
#### `gce`

**Type:** GCE persistent disks

**Configuration options:**

- `disk-type`: Disk type. Valid values: `pd-standard` (default), `pd-ssd`.

(gce-appendix-example-workflows)=
## Appendix: Example workflows

(gce-appendix-workflow-1)=
### Authenticate with a service account (recommended)

**Requirements:**

- Juju 3.6+
- A service account with sufficient privileges (see `service-account` authentication type above).
- The `add-credential` steps must be run from a jump host in Google Cloud to reach the metadata endpoint.

**Steps:**

1. Run `juju add-credential google`; choose `service-account`; supply the service account email.
2. Bootstrap as usual.

```{tip}
With this workflow you avoid storing credential secrets in either your Juju client or controller. The user running `add-credential`/`bootstrap` doesn't need credential secrets.
```

```{tip}
To configure workload machines to use a different (less privileged) service account, use the `instance-role` constraint. This can be set on the model to apply to all (non-controller) machines.
```

(gce-appendix-workflow-2)=
### Authenticate with a credential and a service account

**Requirements:**

- Juju 3.6+
- A service account with sufficient privileges (see `service-account` authentication type above).

**Steps:**

1. Bootstrap with the arg `--bootstrap-constraints="instance-role=<your-service-account-email>"`.
2. The controller machines will be created and attached to that service account.
3. To use the project's default service account, set `instance-role=auto` instead.

```{tip}
To configure workload machines to use a different (less privileged) service account, use the `instance-role` constraint. This can be set on the model to apply to all (non-controller) machines.
```
