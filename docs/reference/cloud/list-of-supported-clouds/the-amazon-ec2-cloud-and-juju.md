(cloud-ec2)=
# The Amazon EC2 cloud and Juju


<!--To see the older HTG-style doc, see version 35. Note that it may be out-of-date. -->

This document describes details specific to using your existing Amazon EC2 cloud with Juju.

> See more: [Amazon EC2](https://docs.aws.amazon.com/ec2/?icmpid=docs_homepage_featuredsvcs)

When using this cloud with Juju, it is important to keep in mind that it is a (1) machine cloud and (2) not some other cloud.

> See more: {ref}`cloud-differences`

As the differences related to (1) are already documented generically in the rest of the docs, here we record just those that follow from (2).

## Notes on `juju add-cloud`

Type in Juju: `ec2`

Name in Juju: `aws`

## Notes on `juju add-credential`


### Authentication types

#### `instance-role`
Attributes:
- `instance-profile-name`: The AWS Instance Profile name (required)

#### `access-key`
Attributes:
- `access-key`: The EC2 access key (required)
- `secret-key`: The EC2 secret key (required)


## Notes on `juju bootstrap`


You can authenticate the controller with the cloud using instance profiles: Use the cloud CLI to create an instance profile, then pass the instance profile to the controller during bootstrap via the `instance-role` constraint: `juju bootstrap --bootstrap-constraints="instance-role=<my instance profile>"`.  See more: `instance-role` below or [Discourse \| Using AWS instance profiles with Juju](https://discourse.charmhub.io/t/5185).

## Cloud-specific model configuration keys

### `vpc-id-force`
Force Juju to use the AWS VPC ID specified with vpc-id, when it fails the minimum validation criteria. Not accepted without vpc-id

| | |
|-|-|
| type | `bool` |
| default value | `false` |
| immutable | `true` |
| mandatory | `false` |

### `vpc-id`
Use a specific AWS VPC ID (optional). When not specified, Juju requires a default VPC or EC2-Classic features to be available for the account/region.

Example: `vpc-a1b2c3d4`

| | |
|-|-|
| type | `string` |
| default value | `""` |
| immutable | `true` |
| mandatory | `false` |


## Supported constraints

| {ref}`CONSTRAINT <constraint>`         |                                                     |
|----------------------------------------|-----------------------------------------------------|
| conflicting:                           | `instance-type` vs. `[cores, cpu-power, mem]` |
| supported?                             |                                                     |
| - {ref}`constraint-allocate-public-ip` | &#10003;                                            |
| - {ref}`constraint-arch`               | &#10003;                                            |
| - {ref}`constraint-cores`              | &#10003;                                            |
| - {ref}`constraint-cpu-power`          | &#10003;                                            |
| - {ref}`constraint-image-id`           | &#10003; <br> An AMI.                               |
| - {ref}`constraint-instance-role`      | &#10005; <br> Value: `auto` or an [instance profile](https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles_use_switch-role-ec2_instance-profiles.html) name.|
| - {ref}`constraint-instance-type`      | &#10003; <br> Valid values: See cloud provider.     |
| - {ref}`constraint-mem`                | &#10003;                                            |
| - {ref}`constraint-root-disk`          | &#10003;                                            |
| - {ref}`constraint-root-disk-source`   | &#10003;                                            |
| - {ref}`constraint-spaces`             | &#10003;                                            |
| - {ref}`constraint-tags`               | &#10005;                                            |
| - {ref}`constraint-virt-type`          | &#10005;                                            |
| - {ref}`constraint-zones`              | &#10003;                                            |

## Supported placement directives

| {ref}`PLACEMENT DIRECTIVE <placement-directive>` ||
|-|-|
| {ref}`placement-directive-machine`               | &#10003; |
| {ref}`placement-directive-subnet`                | &#10003; |
| {ref}`placement-directive-system-id`             | &#10005;   |
| {ref}`placement-directive-zone`                  | &#10003;  <br> If the query looks like a CIDR, then this will match subnets with the same CIDR. If it follows the syntax of a "subnet-XXXX", this will match the Subnet ID. Everything else is just matched as a Name. |

## Cloud-specific storage providers

```{ibnote}
See first: {ref}`storage-provider`
```

(storage-provider-ebs)=
### `ebs`

```{ibnote}
See first: [AWS | EBS volume types](http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/EBSVolumeTypes.html)
```

Prerequisites: Attaching an `ebs` volume to an EC2 instance requires that they both reside within the same availability zone; if this is not the case, Juju will return an error.

Configuration options:

- `volume-type`: Specifies the EBS volume type to create. You can use either the EBS volume type names, or synonyms defined by Juju (in parentheses). Note: Juju's default pool (also called `ebs`) uses `gp2/ssd` as its own default.
    - `standard` (`magnetic`)
    - `gp2` (`ssd`)
    - `gp3`
    - `io1` (`provisioned-iops`)
    - `io2`
    - `st1` (`optimized-hdd`)
    - `sc1` (`cold-storage`)

- `iops`: The number of IOPS for `io1`, `io2` and `gp3` volume types. There are restrictions on minimum and maximum IOPS, as a ratio of the size of volumes. See [Provisioned IOPS (SSD) Volumes](https://docs.aws.amazon.com/ebs/latest/userguide/provisioned-iops.html) for more information.

- `encrypted`: Boolean (true|false); indicates whether created volumes are encrypted.

- `kms-key-id`: The KMS Key ARN used to encrypt the disk. Requires `encrypted: true` to function.

- `throughput`: The number of megabyte/s throughput a GP3 volume is provisioned for. Values are passed in the form `1000M` or `1G` etc.

