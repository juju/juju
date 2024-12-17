(cloud-equinix)=
# The Equinix Metal cloud and Juju


This document describes details specific to using your existing Equinix Metal cloud with Juju. 

> See more: [Equinix Metal](https://deploy.equinix.com/developers/docs/metal/) 

When using this cloud with Juju, it is important to keep in mind that it is a (1) machine cloud and (2) not some other cloud.

> See more: {ref}`cloud-differences`

As the differences related to (1) are already documented generically in the rest of the docs, here we record just those that follow from (2).


## Notes on `juju add-cloud`

Type in Juju: `equinix`

Name in Juju: `equinix`

## Notes on `juju add-credential`


### Authentication types


#### `access-key`
Attributes:
- project-id: Packet project ID (required)
- api-token: Packet API token (required)

<!--
## Notes on `juju bootstrap`


## Cloud-specific model configuration keys
-->


## Supported constraints

| {ref}`CONSTRAINT <constraint>`         |          |
|----------------------------------------|----------|
| conflicting:                           |          |
| supported?                             |          |
| - {ref}`constraint-allocate-public-ip` | TBA      |
| - {ref}`constraint-arch`               | TBA      |
| - {ref}`constraint-container`          | TBA      |
| - {ref}`constraint-cores`              | TBA      |
| - {ref}`constraint-cpu-power`          | TBA      |
| - {ref}`constraint-image-id`           | &#10005; |
| - {ref}`constraint-instance-role`      | TBA      |
| - {ref}`constraint-instance-type`      | TBA      |
| - {ref}`constraint-mem`                | TBA      |
| - {ref}`constraint-root-disk`          | TBA      |
| - {ref}`constraint-root-disk-source`   | &#10005; |
| - {ref}`constraint-spaces`             | &#10005; |
| - {ref}`constraint-tags`               | TBA      |
| - {ref}`constraint-virt-type`          | TBA      |
| - {ref}`constraint-zones`              |          |

## Supported placement directives

| {ref}`PLACEMENT DIRECTIVE <placement-directive>` |          |
|--------------------------------------------------|----------|
| {ref}`placement-directive-machine`               | TBA      |
| {ref}`placement-directive-subnet`                | &#10005; |
| {ref}`placement-directive-system-id`             | &#10005; |
| {ref}`placement-directive-zone`                  | TBA      |


## Other notes

**Before deploying workloads to Equinix metal:** <br> Due to substrate limitations, the Equinix provider does not implement support for firewalls. As a result, workloads deployed to machines under the same project ID can reach each other even across Juju models. Deployed machines are always assigned both a public and a private IP address. This means that any deployed charms are implicitly exposed and proper access control mechanisms need to be implemented to prevent unauthorized access to the deployed workloads.
