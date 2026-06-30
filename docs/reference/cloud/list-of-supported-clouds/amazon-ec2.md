---
myst:
  html_meta:
    description: "Configure and use Amazon EC2 cloud with Juju, including authentication types, instance roles, and machine cloud-specific requirements."
---

(cloud-ec2)=
# Amazon EC2

In Juju, [Amazon EC2](https://docs.aws.amazon.com/ec2/?icmpid=docs_homepage_featuredsvcs) is a {ref}`machine cloud <machine-cloud>` and works as described below.

```{note}
This reference assumes basic familiarity with Juju. If you are new to Juju, start with the {ref}`Tutorial <tutorial>`, then use this page together with the generic materials it links to and/or consult the {ref}`example workflows <ec2-appendix-example-workflows>`.
```

(ec2-limitations)=
## Limitations

- **Single NIC per machine**: EC2 machines are provisioned with one network interface. If your deployment relies on network isolation across multiple subnets, be aware that Juju will connect a machine to one subnet only. See {ref}`ec2-machine-networking-behavior`.

(ec2-cloud-requirements-iam)=
## Requirements

Juju needs Service Account Key Admin, Compute Instance Admin, and Compute Security Admin to create and manage the EC2 resources used during cloud registration and bootstrap.

(ec2-cloud-concepts)=
## Concepts

The following table shows how EC2 abstractions map to Juju concepts:

| Amazon EC2 | Juju |
| - | - |
| [EC2 instance](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/concepts.html) | {ref}`machine <machine>` |
| Process on an instance | {ref}`unit <unit>` |
| Set of instances for one workload | {ref}`application <application>` |
| [EBS volume](https://docs.aws.amazon.com/ebs/latest/userguide/what-is-ebs.html) | {ref}`storage <storage>` |
| [VPC/subnet](https://docs.aws.amazon.com/vpc/latest/userguide/what-is-amazon-vpc.html) | Network spaces and placement targets (roughly) |
| [IAM instance profile](https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles_use_switch-role-ec2_instance-profiles.html) | Cloud identity used by controller and machines |

(ec2-cloud)=
## The cloud

```{ibnote}
See also: {ref}`cloud`, {ref}`Juju | Manage clouds <manage-clouds>`, {ref}`Terraform Provider for Juju | Manage clouds <tfjuju:manage-clouds>`
```

As for all machine clouds, the cloud is registered in Juju via a cloud definition, stored in `clouds.yaml` on the client (on Linux: `~/.local/share/juju/clouds.yaml`) and following this schema:

```yaml
clouds:
  aws:                             # Predefined name for Amazon EC2
    type: ec2
    auth-types:
      - <auth-type>                # See Authentication types below
    regions:
      <region-name>:               # e.g. us-east-1
        endpoint: <endpoint>       # Region-specific EC2 API endpoint
    config:                        # Optional: model config defaults
      <config-key>: <value>        # See Configuration keys below
```

(ec2-credential)=
## Credentials

```{ibnote}
See also: {ref}`credential`, {ref}`Juju | Manage credentials <manage-credentials>`, {ref}`Terraform Provider for Juju | Manage credentials <tfjuju:manage-credentials>`
```

As for all machine clouds, credentials are stored in `credentials.yaml` on the client and follow this schema:

```yaml
credentials:
  aws:                             # Predefined cloud name for Amazon EC2
    <credential-name>:             # User-defined credential name
      auth-type: <auth-type>       # See Authentication types below
      <attribute>: <value>         # Auth-type-specific attributes (see below)
```

(ec2-credential-authentication-types)=
### Authentication types

Amazon EC2 supports the following authentication types:

(ec2-credential-instance-role)=
#### `instance-role`

Attributes:

- `instance-profile-name`: The AWS Instance Profile name (required).

Bootstrap with `juju bootstrap --bootstrap-constraints="instance-role=<profile-name>"`. Use `instance-role=auto` to auto-create the role and profile.

```{ibnote}
See more: [Discourse | Using AWS instance profiles with Juju](https://discourse.charmhub.io/t/5185)
```

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

The controller runs on an EC2 instance provisioned using the same mechanisms as workload machines -- see {ref}`ec2-machine-resources-created-per-machine` for the full per-machine resource model. Controller-specific differences are noted below.

**Compute**

- **EC2 instance**: Ubuntu LTS compute instance. Instance type selected based on hardware constraints (default `m3.medium`). IMDSv2 enforced.
- **IAM Role/Instance Profile** (optional): Created if bootstrap constraints specify `instance-role=auto`. Grants the permissions needed for controller operations and is attached to the instance after launch.

**Networking**

- **Security groups**:
  - Model-wide group: `juju-<model-uuid>`. Initial ingress rules (self-referencing -- source is the group itself):
    - TCP ports 0--65535
    - UDP ports 0--65535
    - ICMP (all types)
  - Machine or global group (no Juju-managed initial rules; rules added via `open-ports`):
    - `firewall-mode=instance` (default): `juju-<model-uuid>-<machine-id>`
    - `firewall-mode=global`: `juju-<model-uuid>-global`
  - Machine/global group also receives ICMPv6 rules from `::/0` per RFC 4890 (Packet Too Big, Time Exceeded, Parameter Problem, Echo Request).
  - All groups tagged with `juju-controller=<controller-uuid>` and `juju-model-uuid=<model-uuid>`.
- **Network interfaces**: Primary interface in specified or default subnet. Public IP optional.

**Storage**

- **EBS root volume**: Device `/dev/sda1`, default 32 GiB (controllers), type `gp2`. Configurable encryption, IOPS, throughput.

(ec2-model)=
## Models

```{ibnote}
See also: {ref}`model`, {ref}`Juju | Manage models <manage-models>`, {ref}`Terraform Provider for Juju | Manage models <tfjuju:manage-models>`
```

(ec2-model-configuration-keys)=
### Configuration keys

Amazon EC2 supports the following {ref}`cloud-specific model configuration keys <model-config-cloud-specific-key>`:

**Networking**

(ec2-model-vpc-id)=
- **`vpc-id`**: Use a specific AWS VPC ID. When not specified, Juju requires a default VPC or EC2-Classic features to be available for the account/region. Example: `vpc-a1b2c3d4`. Type: `string`. Default: `""`. Immutable.

(ec2-model-vpc-id-force)=
- **`vpc-id-force`**: Force Juju to use the AWS VPC ID specified with `vpc-id`, when it fails the minimum validation criteria. Not accepted without `vpc-id`. Type: `bool`. Default: `false`. Immutable.

(ec2-machine)=
## Machines

```{ibnote}
See also: {ref}`machine`, {ref}`Juju | Manage machines <manage-machines>`, {ref}`Terraform Provider for Juju | Manage machines <tfjuju:manage-machines>`
```

(ec2-machine-constraints)=
### Constraints

Amazon EC2 supports the following {ref}`constraints <constraint>`:

```{note}
The constraints `instance-type` and `[arch, cores, cpu-power, mem]` are mutually exclusive, unless `arch` matches the instance type's architecture, in which case they can be combined.
```

**Compute**

- {ref}`constraint-arch`
- {ref}`constraint-container`
- {ref}`constraint-cores`
- {ref}`constraint-cpu-power`
- {ref}`constraint-image-id`. Starting with Juju 3.3. Valid values: An AMI.
- {ref}`constraint-instance-role`. Values: `auto` (creates role automatically) or an [instance profile](https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles_use_switch-role-ec2_instance-profiles.html) name.
- {ref}`constraint-instance-type`. Valid values: Any EC2 instance type. Default: `m3.medium`.
- {ref}`constraint-mem`
**Networking**

- {ref}`constraint-spaces`
- {ref}`constraint-zones`

**Storage**

- {ref}`constraint-root-disk`
- {ref}`constraint-root-disk-source`

(ec2-machine-placement-directives)=
### Placement directives

Amazon EC2 supports the following {ref}`placement directives <placement-directive>`:

- {ref}`placement-directive-machine`
- {ref}`placement-directive-subnet`. If the query looks like a CIDR, matches subnets with the same CIDR. If it follows syntax `subnet-XXXX`, matches the Subnet ID. Otherwise matches subnet Name tag.
- {ref}`placement-directive-zone`

(ec2-machine-resources-created-per-machine)=
### Resources created per machine

Applies to all machines, including controller machines. Controller-specific defaults (e.g. root disk 32 GiB vs. 16 GiB for workload machines) are documented in {ref}`ec2-controller-resources-created-at-bootstrap`.

**Compute**

- **EC2 instance**: Compute instance with name `juju-<model-uuid>-<machine-id>`. Instance type selected based on constraints. IMDSv2 enforced (`HttpTokens=Required`).
- **IAM instance profile** (optional): Attached after launch if `instance-role` constraint specified. Enables EC2 metadata service credentials.

**Networking**

- **Security groups**:
  - Model-wide group: `juju-<model-uuid>`
  - Machine-specific group (`firewall-mode=instance`, default): `juju-<model-uuid>-<machine-id>`
  - Global group (`firewall-mode=global`): `juju-<model-uuid>-global`
- **Network interfaces**: Primary interface in chosen subnet. Private IPs assigned from subnet CIDR. Public IP optional via `MapPublicIPOnLaunch` or `allocate-public-ip` constraint.

**Storage**

- **EBS root volume**: Device `/dev/sda1`, default 16 GiB (applications) or 32 GiB (controllers), type `gp2`. Configurable volume type, encryption, IOPS, throughput.
- **Additional EBS volumes** (optional): Created when storage specified via storage constraints. Must reside in same availability zone as instance.

**Metadata tags:** `juju-model-uuid`, `juju-controller-uuid`, `juju-machine-id`, base OS, architecture.

(ec2-machine-networking-behavior)=
### Networking behavior

- **Spaces:** EC2 machines are provisioned with a single network device. At this time, specifying multiple space constraints and/or endpoint bindings will result in selection of a *single intersecting* space to provision the machine, rather than provisioning multiple NICs.
- **VPC requirements**: When using a VPC, Juju validates the configuration before bootstrap. A valid VPC must have: state `available`; Internet Gateway attached; main route table with default route (`0.0.0.0/0`) to the Internet Gateway; at least one subnet with `MapPublicIPOnLaunch=true`; all subnets using the main route table (not per-subnet route tables). Use `vpc-id-force=true` to skip validation.
- **VPC/subnet selection**: Uses VPC configured via `vpc-id` model config or default VPC. Subnet selection driven by `zone` or `subnet` placement directives. Random selection from available subnets in chosen availability zone. Prefers dual-stack (IPv4+IPv6) subnets.
- **Public IP handling**: Assigned via subnet's `MapPublicIPOnLaunch` setting or `allocate-public-ip` constraint. Can be dissociated after launch if constraint set to `false`.
- **Security groups**: Juju group allows internal model traffic (TCP/UDP 0-65535, ICMP). Machine or global group allows user-defined port rules via `open-ports`.
- **Address resolution**: Returns private (cloud-local), public (if assigned), and IPv6 (if assigned) addresses.
- **Space-aware networking**: Supports space constraints for subnet selection.

(ec2-machine-storage-behavior)=
### Storage behavior

```{ibnote}
See also: {ref}`storage-provider-ebs` for the EBS storage provider configuration options.
```

- **Root disk**: EBS volume, device `/dev/sda1`. Default size 16 GiB for workload machines, 32 GiB for controllers. Type `gp2` by default; configurable via `root-disk-source` constraint (specify a storage pool). Encryption, IOPS, and throughput are also configurable via the pool.
- **Additional volumes**: Created when storage is specified via storage constraints. Must reside in the same availability zone as the instance.
- **AZ constraint**: All EBS volumes (root and additional) must reside in the same availability zone as the EC2 instance.

(ec2-storage)=
## Storage

```{ibnote}
See also: {ref}`storage`, {ref}`Juju | Manage storage <manage-storage>`
```

(ec2-storage-providers)=
### Storage providers

In addition to generic storage providers, Amazon EC2 provides the following {ref}`cloud-specific storage providers <storage-provider-cloud-specific>`:

(storage-provider-ebs)=
#### `ebs`

**Type:** EBS block volumes

```{ibnote}
See more: [AWS | EBS volume types](http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/EBSVolumeTypes.html)
```

**Configuration options:**

- `volume-type`: EBS volume type to create. Valid values: `standard` (`magnetic`), `gp2` (`ssd`), `gp3`, `io1` (`provisioned-iops`), `io2`, `st1` (`optimized-hdd`), `sc1` (`cold-storage`). Juju's default pool uses `gp2`.
- `iops`: The number of IOPS for `io1`, `io2`, and `gp3` volume types. See [Provisioned IOPS (SSD) Volumes](https://docs.aws.amazon.com/ebs/latest/userguide/provisioned-iops.html) for restrictions.
- `encrypted`: Boolean (`true` or `false`). Indicates whether created volumes are encrypted.
- `kms-key-id`: The KMS Key ARN used to encrypt the disk. Requires `encrypted: true`.
- `throughput`: The number of megabytes/s throughput for GP3 volumes. Values: `1000M`, `1G`, etc.

(ec2-appendix-example-workflows)=
## Appendix: Example workflows

(ec2-appendix-quickstart)=
### Add cloud, add credential, bootstrap

1. On an EC2 jump host with an attached IAM role, add or confirm the predefined cloud with `juju add-cloud`.
2. Add credentials with `juju add-credential aws` and choose `instance-role` (recommended; avoids static AWS keys in Juju).
3. Bootstrap with `juju bootstrap --bootstrap-constraints="instance-role=auto" aws aws-controller`.
