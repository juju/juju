---
myst:
  html_meta:
    description: "Configure and use Amazon EC2 cloud with Juju, including authentication types, instance roles, and machine cloud-specific requirements."
---

(cloud-ec2)=
# Amazon EC2

In Juju, [Amazon EC2](https://docs.aws.amazon.com/ec2/?icmpid=docs_homepage_featuredsvcs) is a {ref}`machine cloud <machine-cloud>`. It behaves like all machine clouds, except for a few points of variation related to the cloud, credentials, controllers, models, machines, and storage, described below.

(ec2-cloud)=
## Cloud definition

```{ibnote}
See also: {ref}`cloud`, {ref}`Juju | Manage clouds <manage-clouds>`, {ref}`Terraform Provider for Juju | Manage clouds <tfjuju:manage-clouds>`
```

(ec2-cloud-requirements)=
### Requirements

(ec2-cloud-vpc-requirements)=
#### VPC requirements

When using a VPC, Juju validates the configuration before bootstrap. A valid VPC must have:

- State: `available`.
- Internet Gateway attached.
- Main route table with default route (`0.0.0.0/0`) to the Internet Gateway.
- At least one subnet with `MapPublicIPOnLaunch=true`.
- All subnets using the main route table (not per-subnet route tables).

Use `vpc-id-force=true` to skip validation.

(ec2-cloud-definition)=
### Definition

Type in Juju: `ec2`

Name in Juju: `aws` (predefined)

(ec2-credential)=
## Credentials

```{ibnote}
See also: {ref}`credential`, {ref}`Juju | Manage credentials <manage-credentials>`, {ref}`Terraform Provider for Juju | Manage credentials <tfjuju:manage-credentials>`
```

(ec2-credential-authentication-types)=
### Authentication types

Amazon EC2 supports the following authentication types:

(ec2-credential-instance-role)=
#### `instance-role`

Attributes:

- `instance-profile-name`: The AWS Instance Profile name (required).

(ec2-credential-access-key)=
#### `access-key`

Attributes:

- `access-key`: The EC2 access key (required).
- `secret-key`: The EC2 secret key (required).

(ec2-controller)=
## Controllers

```{ibnote}
See also: {ref}`controller`, {ref}`Juju | Manage controllers <manage-controllers>`, {ref}`Terraform Provider for Juju | Manage controllers <tfjuju:manage-controllers>`
```

(ec2-controller-bootstrap-behavior)=
### Bootstrap behavior

Creates a controller instance on EC2 in a single API request. Juju creates the required EC2 resources directly -- no CloudFormation templates.

(ec2-controller-resources-created-at-bootstrap)=
### Resources created at bootstrap

- **EC2 instance**: Ubuntu LTS compute instance. Instance type selected based on hardware constraints (default `m3.medium`). IMDSv2 enforced.
- **Security groups**:
  - Juju group: `juju-<model-uuid>` (model-wide, allows internal traffic)
  - Machine or global group: `juju-<model-uuid>-<machine-id>` or `juju-<model-uuid>-global` based on `firewall-mode` config
  - Rules: TCP/UDP 0-65535 + ICMP/ICMPv6 ingress from Juju group to itself.
  - Tagged with `juju-controller=<uuid>` and `juju-model-uuid=<uuid>`
- **Network interfaces**: Primary interface in specified or default subnet. Public IP optional.
- **EBS root volume**: Device `/dev/sda1`, default 32 GiB (controllers), type `gp2`. Configurable encryption, IOPS, throughput.
- **IAM Role/Instance Profile** (optional): Created if bootstrap constraints specify `instance-role=auto`. Grants the permissions needed for controller operations and is attached to the instance after launch.

(ec2-controller-other)=
### Other

(ec2-controller-iam-instance-roles)=
#### IAM instance roles

Bootstrap with instance profiles using `juju bootstrap --bootstrap-constraints="instance-role=<profile-name>"`. Use `instance-role=auto` to auto-create role and profile.

```{ibnote}
See more: [Discourse | Using AWS instance profiles with Juju](https://discourse.charmhub.io/t/5185)
```

(ec2-model)=
## Models

```{ibnote}
See also: {ref}`model`, {ref}`Juju | Manage models <manage-models>`, {ref}`Terraform Provider for Juju | Manage models <tfjuju:manage-models>`
```

(ec2-model-configuration-keys)=
### Configuration keys

Amazon EC2 supports the following {ref}`cloud-specific model configuration keys <model-config-cloud-specific-key>`:

(ec2-model-vpc-id)=
#### `vpc-id`

Use a specific AWS VPC ID. When not specified, Juju requires a default VPC or EC2-Classic features to be available for the account/region. Example: `vpc-a1b2c3d4`.

- **Type**: `string`
- **Default value**: `""`
- **Immutable**: `true`
- **Mandatory**: `false`

(ec2-model-vpc-id-force)=
#### `vpc-id-force`

Force Juju to use the AWS VPC ID specified with `vpc-id`, when it fails the minimum validation criteria. Not accepted without `vpc-id`.

- **Type**: `bool`
- **Default value**: `false`
- **Immutable**: `true`
- **Mandatory**: `false`

(ec2-machine)=
## Machines

```{ibnote}
See also: {ref}`machine`, {ref}`Juju | Manage machines <manage-machines>`, {ref}`Terraform Provider for Juju | Manage machines <tfjuju:manage-machines>`
```

(ec2-machine-constraints)=
### Constraints

Amazon EC2 supports the following constraints:

```{note}
The constraints `instance-type` and `[cores, cpu-power, mem]` are mutually exclusive.
```

- {ref}`constraint-allocate-public-ip`
- {ref}`constraint-arch`
- {ref}`constraint-container`
- {ref}`constraint-cores`
- {ref}`constraint-cpu-power`
- {ref}`constraint-image-id`. Starting with Juju 3.3. Valid values: An AMI.
- {ref}`constraint-instance-role`. Values: `auto` (creates role automatically) or an [instance profile](https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles_use_switch-role-ec2_instance-profiles.html) name.
- {ref}`constraint-instance-type`. Valid values: Any EC2 instance type. Default: `m3.medium`.
- {ref}`constraint-mem`
- {ref}`constraint-root-disk`
- {ref}`constraint-root-disk-source`
- {ref}`constraint-spaces`
- {ref}`constraint-zones`

(ec2-machine-placement-directives)=
### Placement directives

Amazon EC2 supports the following placement directives:

- {ref}`placement-directive-machine`
- {ref}`placement-directive-subnet`. If the query looks like a CIDR, matches subnets with the same CIDR. If it follows syntax `subnet-XXXX`, matches the Subnet ID. Otherwise matches subnet Name tag.
- {ref}`placement-directive-zone`

(ec2-machine-resources-created-per-machine)=
### Resources created per machine

Each machine (controller or application) receives:

- **EC2 instance**: Compute instance with name `juju-<model-uuid>-<machine-id>`. Instance type selected based on constraints. IMDSv2 enforced (`HttpTokens=Required`).
- **Security group memberships**: Juju group (model-wide) plus machine-specific or global group based on `firewall-mode`.
- **Network interfaces**: Primary interface in chosen subnet. Private IPs assigned from subnet CIDR. Public IP optional via `MapPublicIPOnLaunch` or `allocate-public-ip` constraint.
- **EBS root volume**: Device `/dev/sda1`, default 16 GiB (applications) or 32 GiB (controllers), type `gp2`. Configurable volume type, encryption, IOPS, throughput.
- **IAM instance profile** (optional): Attached after launch if `instance-role` constraint specified. Enables EC2 metadata service credentials.
- **Additional EBS volumes** (optional): Created when storage specified via storage constraints. Must reside in same availability zone as instance.

**Instance metadata tags:** `juju-model-uuid`, `juju-controller-uuid`, `juju-machine-id`, base OS, architecture.

(ec2-machine-networking-behavior)=
### Networking behavior

- **VPC/subnet selection**: Uses VPC configured via `vpc-id` model config or default VPC. Subnet selection driven by `zone` or `subnet` placement directives. Random selection from available subnets in chosen availability zone. Prefers dual-stack (IPv4+IPv6) subnets.
- **Public IP handling**: Assigned via subnet's `MapPublicIPOnLaunch` setting or `allocate-public-ip` constraint. Can be dissociated after launch if constraint set to `false`.
- **Security groups**: Juju group allows internal model traffic (TCP/UDP 0-65535, ICMP). Machine or global group allows user-defined port rules via `open-ports`.
- **Address resolution**: Returns private (cloud-local), public (if assigned), and IPv6 (if assigned) addresses.
- **Space-aware networking**: Supports space constraints for subnet selection.

(ec2-storage)=
## Storage

```{ibnote}
See also: {ref}`storage`, {ref}`Juju | Manage storage <manage-storage>`
```

### Storage providers

In addition to generic storage providers, Amazon EC2 provides the following {ref}`cloud-specific storage providers <storage-provider-cloud-specific>`:

(storage-provider-ebs)=
#### `ebs`

**Type:** EBS block volumes

```{ibnote}
See more: [AWS | EBS volume types](http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/EBSVolumeTypes.html)
```

**Prerequisites:** EBS volumes must reside in the same availability zone as the EC2 instance.

**Configuration options:**

- `volume-type`: EBS volume type to create. Valid values: `standard` (`magnetic`), `gp2` (`ssd`), `gp3`, `io1` (`provisioned-iops`), `io2`, `st1` (`optimized-hdd`), `sc1` (`cold-storage`). Juju's default pool uses `gp2`.
- `iops`: The number of IOPS for `io1`, `io2`, and `gp3` volume types. See [Provisioned IOPS (SSD) Volumes](https://docs.aws.amazon.com/ebs/latest/userguide/provisioned-iops.html) for restrictions.
- `encrypted`: Boolean (`true` or `false`). Indicates whether created volumes are encrypted.
- `kms-key-id`: The KMS Key ARN used to encrypt the disk. Requires `encrypted: true`.
- `throughput`: The number of megabytes/s throughput for GP3 volumes. Values: `1000M`, `1G`, etc.

