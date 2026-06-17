---
myst:
  html_meta:
    description: "Configure and use Equinix Metal cloud with Juju, including authentication types and machine cloud-specific requirements."
---

(cloud-equinix)=
# Equinix Metal

In Juju, [Equinix Metal](https://deploy.equinix.com/developers/docs/metal/) is a {ref}`machine cloud <machine-cloud>`. It behaves like all machine clouds, except for a few points of variation related to the cloud, credentials, controllers, models, machines, and storage, described below.

```{note}
This reference assumes basic familiarity with Juju. If you are new to Juju, start with the {ref}`tutorial`, then use this page together with the generic materials it links to. For a cloud-specific starting point, see {ref}`equinix-appendix-example-workflows`.
```

(equinix-cloud-limitations)=
## Limitations

#### Firewall limitations

Equinix Metal does not implement firewall support. As a result:

- Workloads deployed to machines under the same project ID can reach each other even across Juju models.
- Deployed machines are always assigned both a public and a private IP address.
- Any deployed charms are implicitly exposed.
- Proper access control mechanisms need to be implemented at the application level to prevent unauthorized access to deployed workloads.

(equinix-cloud-concepts)=
## Concepts

The following table shows how Equinix Metal abstractions map to Juju concepts:

| Equinix Metal | Juju |
| - | - |
| Provisioned server | {ref}`machine <machine>` |
| Process on a server | {ref}`unit <unit>` |
| Group of units for one workload | {ref}`application <application>` |
| Attached block storage (if used) | {ref}`storage <storage>` |
| Facility/metro zone | Placement target (`zones`) |
| Project ID and API token | Cloud access boundary and credential |

(equinix-cloud)=
## The cloud

```{ibnote}
See also: {ref}`cloud`, {ref}`Juju | Manage clouds <manage-clouds>`, {ref}`Terraform Provider for Juju | Manage clouds <tfjuju:manage-clouds>`
```

Type in Juju: `equinix`

Name in Juju: `equinix`

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

Equinix Metal supports the following {ref}`constraints <constraint>`:

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

(equinix-appendix-example-workflows)=
## Appendix: Example workflows

(equinix-appendix-quickstart)=
### Add cloud, add credential, bootstrap

1. Add or confirm the predefined cloud with `juju add-cloud`.
2. Add credentials with `juju add-credential equinix` and choose `access-key`.
3. Bootstrap with `juju bootstrap equinix equinix-controller`.
