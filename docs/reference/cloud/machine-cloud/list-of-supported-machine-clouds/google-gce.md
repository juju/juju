---
myst:
  html_meta:
    description: "Configure Google Compute Engine (GCE) cloud with Juju, including IAM permissions, service account setup, and authentication types."
---

(cloud-gce)=
# Google GCE

In Juju, [Google GCE](https://cloud.google.com/compute/docs) is a {ref}`machine cloud <machine-cloud>`. It behaves like all machine clouds, except for a few points of variation related to the cloud, credentials, controllers, models, machines, and storage, described below.

```{note}
This reference assumes basic familiarity with Juju. If you are new to Juju, start with the {ref}`tutorial`, then use this page together with the generic materials it links to. For a cloud-specific starting point, see {ref}`gce-appendix-example-workflows`.
```

(gce-cloud-requirements)=
## Requirements

#### IAM permissions

Juju needs Service Account Key Admin, Compute Instance Admin, and Compute Security Admin to create and manage the GCE resources used during cloud registration and bootstrap.

```{ibnote}
See more: [Google | Compute Engine IAM roles and permissions](https://cloud.google.com/compute/docs/access/iam)
```

#### VPC requirements

If you use a VPC, Juju validates the configuration before bootstrap. A valid VPC must have:

- At least one subnet with status `READY`, OR.
- `AutoCreateSubnetworks=true` enabled.
- SSH access enabled (firewall rule for port 22).

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

Type in Juju: `gce`

Name in Juju: `google` (predefined)

(gce-credential)=
## Credentials

```{ibnote}
See also: {ref}`credential`, {ref}`Juju | Manage credentials <manage-credentials>`, {ref}`Terraform Provider for Juju | Manage credentials <tfjuju:manage-credentials>`
```

When adding a credential for Google GCE, Juju supports the following authentication types.

**Environment variables (optional):**

- `CLOUDSDK_COMPUTE_REGION`
- `GOOGLE_APPLICATION_CREDENTIALS=<path to JSON credentials file>`

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

- **Compute instance**: Ubuntu LTS instance. Machine type selected based on hardware constraints (default `n1-standard-1`). Instance creation includes boot disk inline (not separate API call).
- **Boot disk**: Persistent disk, device name auto-assigned. Default 10 GiB minimum (expanded if constraint/image requires). Type `pd-standard` (default) or `pd-ssd`. Auto-deleted when instance terminates.
- **Network interface**: Primary interface in specified VPC/subnet or default network. Private IP auto-assigned from subnet CIDR. External NAT with public IP if `allocate-public-ip=true` (default).
- **Firewall rule**: Global rule `juju-<model-uuid>` targeting instances with tag `juju-<model-uuid>`. Allows ingress from instances with same tag.
- **Service account** (optional): Attached if credential type is `service-account` or `instance-role` constraint specified. Scopes: `compute`, `devstorage.full_control`. Enables metadata service credentials.
- **Instance metadata**: Tagged with `juju-controller-uuid`, `juju-is-controller: true`, and bootstrap metadata for controller initialization.
- **Instance tags**: `juju-<model-uuid>` (for firewall targeting), hostname.

(gce-model)=
## Models

```{ibnote}
See also: {ref}`model`, {ref}`Juju | Manage models <manage-models>`, {ref}`Terraform Provider for Juju | Manage models <tfjuju:manage-models>`
```

(gce-model-configuration-keys)=
### Configuration keys

Google GCE supports the following {ref}`cloud-specific model configuration keys <model-config-cloud-specific-key>`:

(gce-model-vpc-id)=
#### `vpc-id`

Use a specific VPC network. When not specified, Juju requires a default VPC to be available for the account. Example: `vpc-a1b2c3d4`.

- **Type**: `string`
- **Default value**: `""`
- **Immutable**: `true`
- **Mandatory**: `false`

(gce-model-vpc-id-force)=
#### `vpc-id-force`

Force Juju to use the GCE VPC ID specified with `vpc-id`, when it fails the minimum validation criteria.

- **Type**: `bool`
- **Default value**: `false`
- **Immutable**: `true`
- **Mandatory**: `false`

(gce-model-base-image-path)=
#### `base-image-path`

Base path to look for machine disk images.

- **Type**: `string`
- **Default value**: (omitted)..
- **Immutable**: `false`
- **Mandatory**: `false`

(gce-machine)=
## Machines

```{ibnote}
See also: {ref}`machine`, {ref}`Juju | Manage machines <manage-machines>`, {ref}`Terraform Provider for Juju | Manage machines <tfjuju:manage-machines>`
```


(gce-machine-constraints)=
### Constraints

Google GCE supports the following {ref}`constraints <constraint>`:

```{note}
The constraints `instance-type` and `[arch, cores, cpu-power, mem]` are mutually exclusive.
```

- {ref}`constraint-allocate-public-ip`
- {ref}`constraint-arch`
- {ref}`constraint-container`
- {ref}`constraint-cores`
- {ref}`constraint-cpu-power`
- {ref}`constraint-instance-role`. Valid values: A service account email.
- {ref}`constraint-instance-type`. Valid values: Any GCE machine type. Default: `n1-standard-1`.
- {ref}`constraint-mem`
- {ref}`constraint-root-disk`
- {ref}`constraint-root-disk-source`
- {ref}`constraint-spaces`
- {ref}`constraint-zones`

(gce-machine-placement-directives)=
### Placement directives

Google GCE supports the following placement directives:

- {ref}`placement-directive-machine`
- {ref}`placement-directive-subnet`: Matches subnet by name or CIDR range.
- {ref}`placement-directive-zone`

(gce-machine-resources-created-per-machine)=
### Resources created per machine

Each machine (controller or application) receives:

- **Compute instance**: Instance with name `<model-uuid><machine-id>`. Machine type selected based on constraints. Status sequence: `PROVISIONING` → `STAGING` → `RUNNING`.
- **Boot disk**: Persistent disk attached inline. Size: max(10 GiB, constraint, image minimum). Type: `pd-standard` (default) or `pd-ssd` via `root-disk-source` constraint. Auto-deleted when instance terminates.
- **Network interface**: Primary interface in VPC/subnet. Private IP auto-assigned. External NAT with public IP if `allocate-public-ip=true`.
- **Service account** (optional): Attached if `instance-role` constraint specified. Enables metadata service credentials.
- **Additional persistent disks** (optional): Created when storage specified via storage constraints. Formatted name `<zone>--<uuid>`. Attached with device name `<zone>-<disk-id>`. Must reside in same zone as instance.
- **Instance metadata**: Bootstrap metadata, controller UUID, and model UUID.
- **Instance tags**: `juju-<model-uuid>`, hostname (for firewall targeting).
- **Disk labels**: `juju-model`, `juju-controller` (set via upgrade step).

(gce-machine-networking-behavior)=
### Networking behavior

- **VPC/subnet selection**: Uses VPC configured via `vpc-id` model config or default network (`global/networks/default`). Subnet selection driven by `zone` or `subnet` placement directives. Random selection from available subnets in region. Space constraints filter to valid subnets.
- **Public IP handling**: Assigned via external NAT (`ONE_TO_ONE_NAT`) if `allocate-public-ip=true` (default). Ephemeral public IP auto-assigned by GCE.
- **Firewall rules**: Environment-level rule (`juju-<model-uuid>`) allows traffic between instances with same tag. Per-machine rules target instance by hostname tag. User-defined port rules via `open-ports` create additional firewall rules.
- **Address resolution**: Returns private address (cloud-local scope, from subnet CIDR) and public address (if NAT configured).

(gce-storage)=
## Storage

```{ibnote}
See also: {ref}`storage`, {ref}`Juju | Manage storage <manage-storage>`
```

### Storage providers

In addition to generic storage providers, Google GCE provides the following {ref}`cloud-specific storage providers <storage-provider-cloud-specific>`:

(storage-provider-gce)=
#### `gce`

**Type:** GCE persistent disks

**Configuration options:**

- `disk-type`: Disk type. Valid values: `pd-standard` (default), `pd-ssd`.

(gce-appendix-example-workflows)=
## Appendix: Example workflows

(gce-appendix-quickstart)=
### Add cloud, add credential, bootstrap

1. On a jump host in Google Cloud, add or confirm the predefined cloud with `juju add-cloud`.
2. Run `juju add-credential google` and choose `service-account` (recommended; avoids storing static key material in Juju).
3. Bootstrap with `juju bootstrap google gce-controller`.

(gce-appendix-workflow-1)=
### Authenticate with a service account (recommended)

**Requirements:**

- Juju 3.6+
- A service account with sufficient privileges (see `service-account` authentication type above)
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
- A service account with sufficient privileges (see `service-account` authentication type above)

**Steps:**

1. Bootstrap with the arg `--bootstrap-constraints="instance-role=<your-service-account-email>"`.
2. The controller machines will be created and attached to that service account.
3. To use the project's default service account, set `instance-role=auto` instead.

```{tip}
To configure workload machines to use a different (less privileged) service account, use the `instance-role` constraint. This can be set on the model to apply to all (non-controller) machines.
```
