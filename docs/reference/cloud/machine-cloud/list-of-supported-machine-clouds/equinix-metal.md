---
myst:
  html_meta:
    description: "Configure and use Equinix Metal cloud with Juju, including authentication types and machine cloud-specific requirements."
---

(cloud-equinix)=
# Equinix Metal

In Juju, [Equinix Metal](https://deploy.equinix.com/developers/docs/metal/) is a {ref}`machine cloud <machine-cloud>`. It behaves like all machine clouds, except for a few points of variation related to the cloud, credentials, controllers, models, machines, and storage, described below.

(equinix-cloud)=
## The cloud

```{ibnote}
See also: {ref}`cloud`, {ref}`Juju | Manage clouds <manage-clouds>`, {ref}`Terraform Provider for Juju | Manage clouds <tfjuju:manage-clouds>`
```

(equinix-cloud-definition)=
### Definition

Type in Juju: `equinix`

Name in Juju: `equinix`

(equinix-cloud-other)=
### Other

(equinix-cloud-firewall-limitations)=
#### Firewall limitations

Equinix Metal does not implement firewall support. As a result:

- Workloads deployed to machines under the same project ID can reach each other even across Juju models.
- Deployed machines are always assigned both a public and a private IP address.
- Any deployed charms are implicitly exposed.
- Proper access control mechanisms need to be implemented at the application level to prevent unauthorized access to deployed workloads.

(equinix-credential)=
## Credentials

```{ibnote}
See also: {ref}`credential`, {ref}`Juju | Manage credentials <manage-credentials>`, {ref}`Terraform Provider for Juju | Manage credentials <tfjuju:manage-credentials>`
```

(equinix-credential-authentication-types)=
### Authentication types

Equinix Metal supports the following authentication type:

(equinix-credential-access-key)=
#### `access-key`

Attributes:

- `project-id`: Equinix Metal project ID (required).
- `api-token`: Equinix Metal API token (required).

(equinix-machine)=
## Machines

```{ibnote}
See also: {ref}`machine`, {ref}`Juju | Manage machines <manage-machines>`, {ref}`Terraform Provider for Juju | Manage machines <tfjuju:manage-machines>`
```

(equinix-machine-constraints)=
### Constraints

Equinix Metal supports the following constraints:

- {ref}`constraint-allocate-public-ip`
- {ref}`constraint-arch`
- {ref}`constraint-cores`
- {ref}`constraint-cpu-power`
- {ref}`constraint-mem`
- {ref}`constraint-zones`

Constraints not listed above are either not supported or automatically determined by the cloud provider.

(equinix-machine-placement-directives)=
### Placement directives

Equinix Metal supports the following placement directive:

- {ref}`placement-directive-zone`
