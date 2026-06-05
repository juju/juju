---
myst:
  html_meta:
    description: "Deploy on Equinix Metal cloud using Juju, including authentication, cloud credentials, and machine cloud-specific configuration."
---

(cloud-equinix)=
# Equinix Metal

In Juju, [Equinix Metal](https://deploy.equinix.com/developers/docs/metal/) is a {ref}`machine cloud <machine-cloud>`. It behaves like all {ref}`machine clouds <machine-cloud>`, except for a few points of variation related to the cloud, credentials, controllers, models, machines, and storage, described below.

## Notes on `juju add-cloud`

Type in Juju: `equinix`

Name in Juju: `equinix`

## Notes on `juju add-credential`

### Authentication types

Equinix Metal supports the following authentication types:

#### `access-key`
Attributes:
- `project-id`: Packet project ID (required).
- `api-token`: Packet API token (required).

## Supported constraints

Equinix Metal supports the following constraints:

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

Equinix Metal supports the following placement directives:

| {ref}`PLACEMENT DIRECTIVE <placement-directive>` |          |
|--------------------------------------------------|----------|
| {ref}`placement-directive-machine`               | TBA      |
| {ref}`placement-directive-subnet`                | &#10005; |
| {ref}`placement-directive-system-id`             | &#10005; |
| {ref}`placement-directive-zone`                  | TBA      |

## Other notes

**Before deploying workloads to Equinix metal:** <br> Due to substrate limitations, the Equinix provider does not implement support for firewalls. As a result, workloads deployed to machines under the same project ID can reach each other even across Juju models. Deployed machines are always assigned both a public and a private IP address. This means that any deployed charms are implicitly exposed and proper access control mechanisms need to be implemented to prevent unauthorized access to the deployed workloads.
