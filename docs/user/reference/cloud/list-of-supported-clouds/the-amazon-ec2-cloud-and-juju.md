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


#### instance-role
Attributes:
- instance-profile-name: The AWS Instance Profile name (required)

#### access-key
Attributes:
- access-key: The EC2 access key (required)
- secret-key: The EC2 secret key (required)


## Notes on `juju bootstrap`


You can authenticate the controller with the cloud using instance profiles: Use the cloud CLI to create an instance profile, then pass the instance profile to the controller during bootstrap via the `instance-role` constraint: `juju bootstrap --bootstrap-constraints="instance-role=<my instance profile>"`.  See more: `instance-role` below or [Discourse \| Using AWS instance profiles with Juju](https://discourse.charmhub.io/t/5185).

## Cloud-specific model configuration keys

### vpc-id-force
Force Juju to use the AWS VPC ID specified with vpc-id, when it fails the minimum validation criteria. Not accepted without vpc-id

| | |
|-|-|
| type | bool |
| default value | false |
| immutable | true |
| mandatory | false |

### vpc-id
Use a specific AWS VPC ID (optional). When not specified, Juju requires a default VPC or EC2-Classic features to be available for the account/region.

Example: vpc-a1b2c3d4
| | |
|-|-|
| type | string |
| default value | "" |
| immutable | true |
| mandatory | false |


## Supported constraints

| {ref}`CONSTRAINT <constraint>`         |                                                                                                                                                                   |
|----------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| conflicting:                           | `instance-type` vs. `[cores, cpu-power, mem]`                                                                                                                     |
| supported?                             |                                                                                                                                                                   |
| - {ref}`constraint-allocate-public-ip` | &#10003;                                                                                                                                                          |
| - {ref}`constraint-arch`               | &#10003;                                                                                                                                                          |
| - {ref}`constraint-container`          | &#10003;                                                                                                                                                          |
| - {ref}`constraint-cores`              | &#10003;                                                                                                                                                          |
| - {ref}`constraint-cpu-power`          | &#10003;                                                                                                                                                          |
| - {ref}`constraint-image-id`           | &#10003;  (Starting with Juju 3.3) <br> Type: String. <br> Valid values: An AMI.                                                                                  |
| - {ref}`constraint-instance-role`      | &#10003;  <br> Value: `auto` or an [instance profile](https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles_use_switch-role-ec2_instance-profiles.html) name. |
| - {ref}`constraint-instance-type`      | &#10003;  <br> Valid values: See cloud provider. <br> Default value: `m3.medium`.                                                                                 |
| - {ref}`constraint-mem`                | &#10003;                                                                                                                                                          |
| - {ref}`constraint-root-disk`          | &#10003;                                                                                                                                                          |
| - {ref}`constraint-root-disk-source`   | &#10003;                                                                                                                                                          |
| - {ref}`constraint-spaces`             | &#10003;                                                                                                                                                          |
| - {ref}`constraint-tags`               | &#10005;                                                                                                                                                          |
| - {ref}`constraint-virt-type`          | &#10005;                                                                                                                                                          |
| - {ref}`constraint-zones`              | &#10003;                                                                                                                                                          |


## Supported placement directives

| {ref}`PLACEMENT DIRECTIVE <placement-directive>` |                                                                                                                                                                                                                        |
|--------------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| {ref}`placement-directive-machine`               | &#10003;                                                                                                                                                                                                               |
| {ref}`placement-directive-subnet`                | &#10003;                                                                                                                                                                                                               |
| {ref}`placement-directive-system-id`             | &#10005;                                                                                                                                                                                                               |
| {ref}`placement-directive-zone`                  | &#10003;  <br> If the query looks like a CIDR, then this will match subnets with the same CIDR. If it follows the syntax of a "subnet-XXXX", this will match the Subnet ID. Everything else is just matched as a Name. |

