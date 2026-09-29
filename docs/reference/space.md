---
myst:
  html_meta:
    description: "Juju network space reference: logical grouping of subnets for traffic segmentation, security, performance, and regulatory compliance. The space record, operations, watchers, and rules."
---

(space)=
# Space

```{ibnote}
See also: {ref}`manage-spaces`
```

In Juju, a **(network) space** is a logical grouping of {ref}`subnets <subnet>` that can communicate with one another, used to segment network traffic for performance, for security, or to control the scope of regulatory compliance.

(the-space-operations)=
## The space in the declaration layer

How clients express network intent: the space record's life as a grouping, its subnet membership, and the bindings and constraints that name spaces.

- **Creation:** Adding a space creates the record; the name must be unique per model.
- **Renaming:** Renaming rewrites the space's name.
- **Removal:** Removing a space first checks what names it: a dry run reports the violations (model and application constraints, application bindings), and without forcing, removal stops while anything still names the space.
- **Subnet membership:** Moving a subnet between spaces rewrites the subnet's space pointer.
- **Provider reload:** The provider can be asked to rediscover its spaces and subnets: where the cloud exposes its own space notion, the reload adopts it; where it does not, everything falls into the default `alpha` space.
- **Bindings:** An {ref}`application <application>` carries its default-binding space on its own record, and each charm endpoint can carry a space binding of its own; the `default-space` model configuration value names the space endpoints bind to by default.
- **Exposed endpoints:** {ref}`Exposed endpoints <the-application-operations>` grant spaces to the outside.
- **Constraints:** {ref}`Constraints <constraint>` can name the spaces a machine's subnets must come from.

(the-space-declaration-rules)=
### Declaration rules and errors

- **Rules:**
  - A space name uses lowercase letters and digits, with hyphens between segments; `alpha` is the default space every model starts with.
  - A space name is unique per model.
  - The `alpha` space cannot be removed.
- **Errors:**
  - **`space already exists`:** Triggered when adding a space whose name is already taken in the model. Remediation: choose an unused name.
  - **`space name is not valid`:** Triggered when a space name fails the name grammar. Remediation: use lowercase letters and digits, with hyphens between segments.
  - **`cannot remove the alpha space`:** Triggered when deleting the `alpha` space. Remediation: none; the alpha space is permanent.
  - **`space requirement conflict`:** Triggered when a space is both a positive and a negative space requirement for a machine (a constraint or endpoint binding naming it and a constraint excluding it). Remediation: resolve the conflicting constraints or bindings.
  - **`space requirements unsatisfiable`:** Triggered when a container's or VM's space requirements cannot be met by its host (no device or bridge available in the required space). Remediation: relax the space constraints or extend the host's networking in the required space.

## The space in the persistence layer

```{ggarch}
:file: ../juju.ggarch
:view: Network spaces
:alt: The space record at the centre with its salient columns; the cloud's provider-space record west; the application's default binding east; the subnets below. Each line is a stored pointer; 1/m at each end.
:caption: The space's stored records. A space is one naming record in the model database, its alpha default seeded at creation; the cloud's own identifier for the same grouping is a provider-space record, at most one per space; a subnet carries the nullable space pointer, so it belongs to 0..1 space; an application carries its default-binding pointer on its own record. Every line is a foreign key in one of those records.
```

A space is persisted in the {ref}`model database <data-model-full-spine>` as follows:

- **The space is one record:** the identity pair, a `uuid` primary key and a model-unique `name`; the unique index rejects a second space of the same name (`space already exists`). The model seeds `alpha`, the default space.
- **The cloud's own identifier for the same grouping is its own record,** where the cloud has one; at most one per space.
- **The subnet carries the space pointer;** the pointer is nullable, so a subnet belongs to exactly one space or to none.
- **The pointers that name it:** An {ref}`application <application>` carries its default-binding space on its own record, each charm endpoint can carry a space binding of its own, and exposed endpoints grant spaces to the outside; {ref}`constraints <constraint>` can name the spaces a machine's subnets must come from.

A space has no state machine and no life column: it is a naming record, created, renamed, or removed; nothing transitions.

(the-space-persistence-rules)=
### Persistence rules and errors

- **Rules:**
  - A subnet belongs to at most one space or to none: the space pointer is nullable.
  - Deleting a space resets what names it: constraints naming it are removed, default bindings and exposed endpoints move to `alpha`, endpoint bindings are dropped, and its subnets move to `alpha`.
- **Errors:**
  - **`space not found`:** Triggered when querying a space by UUID or name that does not exist. Remediation: verify the space exists in the model.
  - **`subnet not found`:** Triggered when the queried subnet, by CIDR or identifier, does not exist. Remediation: verify the subnet exists in the model.

## The space in the execution layer

A space has no machinery of its own: it is a grouping the controller maintains; subnets are moved into it and reloaded from the provider, and the network's watch surface reports what changed.

(the-space-watchers)=
### Space watchers

The network domain exposes one watch surface:

- **Subnet changes:** The surfaces that need the network's shape (the provisioning and address machinery) watch subnets, not spaces; a space's change is implied by its subnets' moves.

Every watcher fires once immediately when it is created, the initial query being the baseline snapshot, and again on each qualifying change. See {ref}`the watcher pattern <watchers>`.

## Support for spaces in Juju providers

Support for spaces may vary from one cloud to another. For cloud-specific details, see the networking behavior section in each cloud's reference doc: {ref}`Amazon EC2 <cloud-ec2>`, {ref}`Microsoft Azure <cloud-azure>`, {ref}`OpenStack <cloud-openstack>`, {ref}`LXD <cloud-lxd>`, {ref}`MAAS <cloud-maas>`, {ref}`Unmanaged <cloud-unmanaged>`.
