(cloud-openstack)=
# The OpenStack cloud and Juju


This document describes details specific to using your existing OpenStack cloud with Juju. 

> See more: [OpenStack](https://www.openstack.org/) 

When using this cloud with Juju, it is important to keep in mind that it is a (1) machine cloud and (2) not some other cloud.

> See more: {ref}`cloud-differences`

As the differences related to (1) are already documented generically in the rest of the docs, here we record just those that follow from (2).



## Supported cloud versions

Any version that supports: <br> - compute v2 (Nova) <br> - network v2 (Neutron) (optional) <br> - volume2 (Cinder) (optional) <br> - identity v2 or v3 (Keystone)

## Notes on `juju add-cloud`

Type in Juju: `openstack`.

Name in Juju: User-defined.

**If you want to use the novarc file (recommended):** <br> Source the OpenStack RC file (`source <path to file>`). This will allow Juju to detect values from preset OpenStack environment variables. Run `add-cloud` in interactive mode and accept the suggested defaults.

## Notes on `juju add-credential`

```{important}

**If you want to use environment variables (recommended):** <br> Source the OpenStack RC file (see above). Run `add-credential` and accept the suggested defaults.

```

### Authentication types

#### `userpass`

Attributes:

- username: The username to authenticate with. (required)
- password: The password for the specified username. (required)
- tenant-name: The OpenStack tenant name. (optional)
- tenant-id: The Openstack tenant ID (optional)
- version: The Openstack identity version (optional)
- domain-name: The OpenStack domain name. (optional)
- project-domain-name: The OpenStack project domain name. (optional)
- user-domain-name: The OpenStack user domain name. (optional)


## Notes on `juju bootstrap`

You will need to create an OpenStack machine metadata. If the metadata is available locally, you can pass it to Juju via `juju bootstrap ... --metadata-source <path to metadata simplestreams`. <br> > See more: {ref}`manage-metadata`. <p> **If your cloud has multiple private networks:** You will need to specify the one that you want the instances to boot from via `juju bootstrap ... --model-default network=<network uuid or name>`. <p> **If your cloud's topology requires that its instances are accessed via floating IP addresses:** Pass the `allocate-public-ip=true` (see constraints below) as a bootstrap constraint.


## Cloud-specific model configuration keys

### external-network
The network label or UUID to create floating IP addresses on when multiple external networks exist.

| | |
|-|-|
| type | string |
| default value | "" |
| immutable | false |
| mandatory | false |

### use-openstack-gbp
Whether to use Neutrons Group-Based Policy

| | |
|-|-|
| type | bool |
| default value | false |
| immutable | false |
| mandatory | false |

### policy-target-group
The UUID of Policy Target Group to use for Policy Targets created.

| | |
|-|-|
| type | string |
| default value | "" |
| immutable | false |
| mandatory | false |

### use-default-secgroup
Whether new machine instances should have the "default" Openstack security group assigned in addition to juju defined security groups.

| | |
|-|-|
| type | bool |
| default value | false |
| immutable | false |
| mandatory | false |

### network
The network label or UUID to bring machines up on when multiple networks exist.

| | |
|-|-|
| type | string |
| default value | "" |
| immutable | false |
| mandatory | false |

## Supported constraints

| {ref}`CONSTRAINT <constraint>`         |                                                                                                |
|----------------------------------------|------------------------------------------------------------------------------------------------|
| conflicting:                           | `{ref}`instance-type]` vs. `[mem, root-disk, cores]`                                           |
| supported?                             |                                                                                                |
| - {ref}`constraint-allocate-public-ip` | &#10003;                                                                                       |
| - {ref}`constraint-arch`               | &#10003;                                                                                       |
| - {ref}`constraint-container`          | &#10003;                                                                                       |
| - {ref}`constraint-cores`              | &#10003;                                                                                       |
| - {ref}`constraint-cpu-power`          | &#10005;                                                                                       |
| - {ref}`constraint-image-id`           | &#10003; (Starting with Juju 3.3) <br> Type: String. <br> Valid values: An OpenStack image ID. |
| - {ref}`constraint-instance-role`      | &#10005;                                                                                       |
| - {ref}`constraint-instance-type`      | &#10003; <br> Valid values: Any (cloud admin) user defined OpenStack flavor.                   |
| - {ref}`constraint-mem`                | &#10003;                                                                                       |
| - {ref}`constraint-root-disk`          | &#10003;                                                                                       |
| - {ref}`constraint-root-disk-source`   | &#10003; <br> `root-disk-source` is either `local` or `volume`.                                |
| - {ref}`constraint-spaces`             | &#10005;                                                                                       |
| - {ref}`constraint-tags`               | &#10005;                                                                                       |
| - {ref}`constraint-virt-type`          | &#10003; <br> Valid values: `[kvm, lxd]`.                                                      |
| - {ref}`constraint-zones`                         | &#10003;                                                                                       |

## Supported placement directives

| {ref}`PLACEMENT DIRECTIVE <placement-directive>` |          |
|--------------------------------------------------|----------|
| - {ref}`placement-directive-machine`             | TBA      |
| - {ref}`placement-directive-subnet`              | &#10005; |
| - {ref}`placement-directive-system-id`           | &#10005; |
| - {ref}`placement-directive-zone`                | &#10003; |

