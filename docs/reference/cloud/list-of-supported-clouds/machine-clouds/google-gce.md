---
myst:
  html_meta:
    description: "Configure Google Compute Engine (GCE) cloud with Juju, including IAM permissions, service account setup, and authentication types."
---

(cloud-gce)=
# Google GCE

In Juju, [Google GCE](https://cloud.google.com/compute/docs) is a {ref}`machine cloud <machine-cloud>`. It behaves like all {ref}`machine clouds <machine-clouds>`, except for a few points of variation related to the cloud, credentials, controllers, models, machines, and storage, described below.

(gce-cloud)=
## The cloud

The Google GCE cloud in Juju.

(gce-cloud-definition)=
### Definition

Type in Juju: `gce`

Name in Juju: `google` (predefined)

(gce-cloud-requirements)=
### Requirements

**IAM permissions:** Service Account Key Admin, Compute Instance Admin, and Compute Security Admin.

```{ibnote}
See more: [Google | Compute Engine IAM roles and permissions](https://cloud.google.com/compute/docs/access/iam)
```

(gce-cloud-other)=
### Other

(gce-cloud-vpc-requirements)=
#### VPC requirements

When using a VPC, Juju validates the configuration before bootstrap. A valid VPC must have:

- At least one subnet with status `READY`, OR
- `AutoCreateSubnetworks=true` enabled
- SSH access enabled (firewall rule for port 22)

(gce-credential)=
## Credentials

Credentials for the Google GCE cloud.

**Environment variables (optional):**

- `CLOUDSDK_COMPUTE_REGION`
- `GOOGLE_APPLICATION_CREDENTIALS=<path to JSON credentials file>`

(gce-credential-authentication-types)=
### Authentication types

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
- A service account with sufficient privileges. See: {ref}`gce-appendix-service-account`
- The `add-credential` steps must be run from a jump host running in Google Cloud to reach the cloud metadata endpoint.

```{ibnote}
See more: {ref}`gce-appendix-workflow-1`
```

(gce-controller)=
## Controllers

Controllers bootstrapped on the Google GCE cloud.

(gce-controller-bootstrap-behavior)=
### Bootstrap behavior

Creates a controller instance on GCE via single `InsertInstanceRequest` API call. Uses imperative GCE API calls to create resources -- no templates.

(gce-controller-resources-created-at-bootstrap)=
### Resources created at bootstrap

- **Compute instance**: Ubuntu LTS instance. Machine type selected based on hardware constraints (default `n1-standard-1`). Instance creation includes boot disk inline (not separate API call).
- **Boot disk**: Persistent disk, device name auto-assigned. Default 10 GiB minimum (expanded if constraint/image requires). Type `pd-standard` (default) or `pd-ssd`. Auto-deleted when instance terminates.
- **Network interface**: Primary interface in specified VPC/subnet or default network. Private IP auto-assigned from subnet CIDR. External NAT with public IP if `allocate-public-ip=true` (default).
- **Firewall rule**: Global rule `juju-<model-uuid>` targeting instances with tag `juju-<model-uuid>`. Allows ingress from instances with same tag.
- **Service account** (optional): Attached if credential type is `service-account` or `instance-role` constraint specified. Scopes: `compute`, `devstorage.full_control`. Enables metadata service credentials.
- **Instance metadata**: Tagged with `juju-controller-uuid`, `juju-is-controller: true`. Cloud-init data stored gzipped+base64 encoded.
- **Instance tags**: `juju-<model-uuid>` (for firewall targeting), hostname.

(gce-controller-other)=
### Other

(gce-controller-service-accounts)=
#### Service accounts

Bootstrap with service accounts using `juju bootstrap --bootstrap-constraints="instance-role=<service-account-email>"`. Use `instance-role=auto` to use project's default service account.

```{ibnote}
See more: {ref}`gce-appendix-service-account`, {ref}`gce-appendix-example-authentication-workflows`
```

(gce-model)=
## Models

Models connected to the Google GCE cloud.

(gce-model-cloud-specific-configuration-keys)=
(gce-model-configuration-keys)=
### Configuration keys

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
- **Default value**: (omitted)
- **Immutable**: `false`
- **Mandatory**: `false`

(gce-machine)=
## Machines

Machines provisioned on the Google GCE cloud.

(gce-machine-supported-constraints)=
(gce-machine-constraints)=
### Constraints

```{note}
The constraints `instance-type` and `[arch, cores, cpu-power, mem]` are mutually exclusive.
```

- {ref}`constraint-allocate-public-ip`
- {ref}`constraint-arch`
- {ref}`constraint-container`
- {ref}`constraint-cores`
- {ref}`constraint-cpu-power`
- {ref}`constraint-instance-type`: Valid values: Any GCE machine type. Default: `n1-standard-1`.
- {ref}`constraint-mem`
- {ref}`constraint-root-disk`
- {ref}`constraint-root-disk-source`
- {ref}`constraint-spaces`
- {ref}`constraint-zones`

(gce-machine-supported-placement-directives)=
(gce-machine-placement-directives)=
### Placement directives

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
- **Instance metadata**: Cloud-init data (gzipped+base64), controller UUID, model UUID.
- **Instance tags**: `juju-<model-uuid>`, hostname (for firewall targeting).
- **Disk labels**: `juju-model`, `juju-controller` (set via upgrade step).

(gce-machine-networking-behavior)=
### Networking behavior

- **VPC/subnet selection**: Uses VPC configured via `vpc-id` model config or default network (`global/networks/default`). Subnet selection driven by `zone` or `subnet` placement directives. Random selection from available subnets in region. Space constraints filter to valid subnets.
- **Public IP handling**: Assigned via external NAT (`ONE_TO_ONE_NAT`) if `allocate-public-ip=true` (default). Ephemeral public IP auto-assigned by GCE.
- **Firewall rules**: Environment-level rule (`juju-<model-uuid>`) allows traffic between instances with same tag. Per-machine rules target instance by hostname tag. User-defined port rules via `open-ports` create additional firewall rules.
- **Address resolution**: Returns private address (cloud-local scope, from subnet CIDR) and public address (if NAT configured).

(gce-storage)=
(gce-storage)=
## Storage

Storage provisioned on the Google GCE cloud.

### Storage providers

(storage-provider-gce)=
### `gce`

**Type:** GCE persistent disks

**Configuration options:**

- `disk-type`: Disk type. Valid values: `pd-standard` (default), `pd-ssd`.

```{caution}
Known issue with `pd-ssd`: See [GitHub issue #20349](https://github.com/juju/juju/issues/20349).
```

(gce-appendix-example-authentication-workflows)=
## Appendix: Example authentication workflows

(gce-appendix-workflow-1)=
### Workflow 1 -- Service account only (recommended)

**Requirements:**

- Juju 3.6+
- A service account with sufficient privileges (see {ref}`gce-appendix-service-account`)
- The `add-credential` steps must be run from a jump host in Google Cloud to reach the metadata endpoint

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
### Workflow 2 -- Bootstrap using normal credential; use service account thereafter

**Requirements:**

- Juju 3.6+
- A service account with sufficient privileges (see {ref}`gce-appendix-service-account`)

**Steps:**

1. Bootstrap with the arg `--bootstrap-constraints="instance-role=auto"`.
2. The controller machines will be created and attached to the project's default service account.
3. Alternatively, specify a different service account instead of `auto`.

```{tip}
To configure workload machines to use a different (less privileged) service account, use the `instance-role` constraint. This can be set on the model to apply to all (non-controller) machines.
```

(gce-appendix-service-account)=
## Appendix: Service account requirements

To configure a service account with the privileges required by Juju, assign the following scopes:

- `https://www.googleapis.com/auth/compute`
- `https://www.googleapis.com/auth/devstorage.full_control`
