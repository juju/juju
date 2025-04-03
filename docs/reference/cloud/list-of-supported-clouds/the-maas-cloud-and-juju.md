(cloud-maas)=
# The MAAS cloud and Juju


This document describes details specific to using your existing MAAS cloud with Juju. 

> See more: [MAAS](https://maas.io/) 

When using this cloud with Juju, it is important to keep in mind that it is a (1) machine cloud and (2) not some other cloud.

> See more: {ref}`cloud-differences`

As the differences related to (1) are already documented generically in the rest of the docs, here we record just those that follow from (2).


## Requirements

Starting with `juju v.3.0`, versions of MAAS <2 are no longer supported.

## Notes on `juju add-cloud`

Type in Juju: `maas`

Name in Juju: User-defined.

## Notes on `juju add-credential`


### Authentication types


#### `oauth1`
Attributes:
- maas-oauth: OAuth/API-key credentials for MAAS (required)

```{note}

`maas-oauth` is your MAAS API key. See more: MAAS | How to add an API key for a user](https://maas.io/docs/how-to-manage-user-accounts#heading--api-key)
```

<!--
## Notes on `juju bootstrap`
-->

<!--
## Cloud-specific model configuration keys
-->

## Supported constraints

| {ref}`CONSTRAINT <constraint>`         |                                                                                                  |
|----------------------------------------|--------------------------------------------------------------------------------------------------|
| conflicting:                           |                                                                                                  |
| supported?                             |                                                                                                  |
| - {ref}`constraint-allocate-public-ip` | &#10005;                                                                                         |
| - {ref}`constraint-arch`               | &#10003; <br> Valid values: See cloud provider.                                                  |
| - {ref}`constraint-container`          | &#10003;                                                                                         |
| - {ref}`constraint-cores`              | &#10003;                                                                                         |
| - {ref}`constraint-cpu-power`          | &#10005;                                                                                         |
| - {ref}`constraint-image-id`           | &#10003; (Starting with Juju 3.2) <br> Type: String. <br> Valid values: An image name from MAAS. |
| - {ref}`constraint-instance-role`      | &#10005;                                                                                         |
| - {ref}`constraint-instance-type`      | &#10005;                                                                                         |
| - {ref}`constraint-mem`                | &#10003;                                                                                         |
| - {ref}`constraint-root-disk`          | &#10003;                                                                                         |
| - {ref}`constraint-root-disk-source`   | &#10005;                                                                                         |
| - {ref}`constraint-spaces`             | &#10003;                                                                                         |
| - {ref}`constraint-tags`               | &#10003;                                                                                         |
| - {ref}`constraint-virt-type`          | &#10005;                                                                                         |
| - {ref}`constraint-zones`              | &#10003;                                                                                         |


## Supported placement directives

| {ref}`PLACEMENT DIRECTIVE <placement-directive>` |                                                                     |
|--------------------------------------------------|---------------------------------------------------------------------|
| - {ref}`placement-directive-machine`             | TBA                                                                 |
| - {ref}`placement-directive-subnet`              | &#10005;                                                            |
| - {ref}`placement-directive-system-id`           | &#10003;                                                            |
| - {ref}`placement-directive-zone`                | &#10003; <br> If there's no '=' delimiter, assume it's a node name. |

