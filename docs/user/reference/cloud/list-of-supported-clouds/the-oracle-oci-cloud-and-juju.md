(cloud-oci)=
# The Oracle OCI cloud and Juju

This document describes details specific to using your existing Oracle OCI cloud with Juju. 

> See more: [Oracle OCI](https://docs.oracle.com/en-us/iaas/Content/home.htm) 

When using this cloud with Juju, it is important to keep in mind that it is a (1) machine cloud and (2) not some other cloud.

> See more: {ref}`cloud-differences`

As the differences related to (1) are already documented generically in the rest of the docs, here we record just those that follow from (2).


## Notes on `juju add-cloud`

Type in Juju: `oci`

Name in Juju: `oracle`

## Notes on `juju add-credential`


### Authentication types


#### `httpsig`
Attributes:
- user: Username OCID (required)
- tenancy: Tenancy OCID (required)
- key: PEM encoded private key (required)
- pass-phrase: Passphrase used to unlock the key (required)
- fingerprint: Private key fingerprint (required)
- region: DEPRECATED: Region to log into (required)

## Notes on `juju bootstrap`

|You have to specify the compartment OCID via the cloud-specific `compartment-id` model configuration key (see below). <br> Example: `juju bootstrap --config compartment-id=<compartment OCID> oracle oracle-controller`.

## Cloud-specific model configuration keys

#### `address-space`
The CIDR block to use when creating default subnets. The subnet must have at least a /16 size.

| | |
|-|-|
| type | string |
| default value | "10.0.0.0/16" |
| immutable | false |
| mandatory | false |

#### `compartment-id`
The OCID of the compartment in which juju has access to create resources.

| | |
|-|-|
| type | string |
| default value | "" |
| immutable | false |
| mandatory | false |


## Supported constraints

| {ref}`CONSTRAINT <constraint>`         |                                               |
|----------------------------------------|-----------------------------------------------|
| conflicting:                           |                                               |
| supported?                             |                                               |
| - {ref}`constraint-allocate-public-ip` | TBA                                           |
| - {ref}`constraint-arch`               | &#x2611; <br> Valid values: `[amd64, arm64]`. |
| - {ref}`constraint-container`          | &#10005;                                      |
| - {ref}`constraint-cores`              | &#10003;                                      |
| - {ref}`constraint-cpu-power`          | &#10003;                                      |
| - {ref}`constraint-image-id`           | &#10005;                                      |
| - {ref}`constraint-instance-role`      | &#10005;                                      |
| - {ref}`constraint-instance-type`      | &#10003;                                      |
| - {ref}`constraint-mem`                | &#10003;                                      |
| - {ref}`constraint-root-disk`          | &#10003;                                      |
| - {ref}`constraint-root-disk-source`   | &#10005;                                      |
| - {ref}`constraint-spaces`             | &#10005;                                      |
| - {ref}`constraint-tags`               | &#10005;                                      |
| - {ref}`constraint-virt-type`          | &#10005;                                      |
| - {ref}`constraint-zones`              | &#10003;                                      |


## Supported placement directives

| {ref}`PLACEMENT DIRECTIVE <placement-directive>` |          |
|--------------------------------------------------|----------|
| - {ref}`placement-directive-machine`               | TBA      |
| - {ref}`placement-directive-subnet`                | &#10005; |
| - {ref}`placement-directive-system-id`             | &#10005; |
| - {ref}`placement-directive-zone`                  | TBA      |
