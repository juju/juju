---
myst:
  html_meta:
    description: "Configure and use Equinix Metal cloud with Juju, including authentication types and machine cloud-specific requirements."
---

(cloud-equinix)=
# Equinix Metal

In Juju, [Equinix Metal](https://deploy.equinix.com/developers/docs/metal/) is a {ref}`machine cloud <machine-cloud>` and works as described below.

```{caution}
[Equinix Metal has been sunsetted](https://docs.equinix.com/metal/).
```

```{note}
This reference assumes basic familiarity with Juju. If you are new to Juju, start with the {ref}`Tutorial <tutorial>`, then use this page together with the generic materials it links to.
```

(equinix-limitations)=
## Limitations

- **No firewall support**: Equinix Metal does not implement firewall support. Workloads deployed to machines under the same project ID can reach each other even across Juju models. Deployed machines are always assigned both a public and a private IP address. Any deployed charms are implicitly exposed. Access control must be implemented at the application level.

(equinix-concepts)=
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

As for all machine clouds, the cloud is registered in Juju via a cloud definition, stored in `clouds.yaml` on the client (on Linux: `~/.local/share/juju/clouds.yaml`) and following this schema:

```yaml
clouds:
  <cloud-name>:  # Predefined name
    type: equinix
    auth-types:
      - <auth-type>                # See Authentication types below
    config:                        # Optional: model config defaults
      <config-key>: <value>        # See Configuration keys below
```


(equinix-credential)=
## Credentials

```{ibnote}
See also: {ref}`credential`, {ref}`Juju | Manage credentials <manage-credentials>`, {ref}`Terraform Provider for Juju | Manage credentials <tfjuju:manage-credentials>`
```

As for all machine clouds, credentials are stored in `credentials.yaml` on the client and follow this schema:

```yaml
credentials:
  equinix                        # Predefined cloud name for Equinix Metal
    <credential-name>:             # User-defined credential name
      auth-type: <auth-type>       # access-key (the only type)
      <attribute>: <value>         # Auth-type-specific attributes (see below)
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

**Compute**

- {ref}`constraint-arch`
- {ref}`constraint-cores`
- {ref}`constraint-cpu-power`
- {ref}`constraint-mem`

**Networking**

- {ref}`constraint-allocate-public-ip`
- {ref}`constraint-zones`

Constraints not listed above are either not supported or automatically determined by the cloud provider.

(equinix-machine-placement-directives)=
### Placement directives

Equinix Metal supports the following {ref}`placement directives <placement-directive>`:

- {ref}`placement-directive-zone`

(equinix-storage)=
## Storage

```{ibnote}
See also: {ref}`storage`, {ref}`Juju | Manage storage <manage-storage>`
```

Equinix Metal has no cloud-specific storage providers.
