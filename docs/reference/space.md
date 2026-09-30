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
(the-space-declaration-rules)=
## The space in the declaration layer

How clients express network intent: the space record's life as a grouping, its subnet membership, and the bindings and constraints that name spaces.

- **Creation:** Adding a space creates the record; the name must be unique per model.
  - *Rule:* A space name uses lowercase letters and digits, with hyphens between segments. `alpha` is the default space every model starts with.
  - *Related errors:*
    - **`space already exists`**: *Trigger:* Adding a space whose name is already taken in the model. *Remediation:* Choose an unused name.
    - **`space name is not valid`**: *Trigger:* A space name that fails the name grammar. *Remediation:* Use lowercase letters and digits, with hyphens between segments.
- **Renaming:** Renaming rewrites the space's name, under the same name rules as creation.
- **Removal:** Removing a space first checks what names it: a dry run reports the violations (model and application constraints, application bindings), and without forcing, removal stops while anything still names the space.
  - *Rule:* The `alpha` space cannot be removed.
  - *Related error:*
    - **`cannot remove the alpha space`**: *Trigger:* Deleting the `alpha` space. *Remediation:* None; the alpha space is permanent.
- **Subnet membership:** Moving a subnet between spaces rewrites the subnet's space pointer.
- **Provider reload:** The provider can be asked to rediscover its spaces and subnets: where the cloud exposes its own space notion, the reload adopts it; where it does not, everything falls into the default `alpha` space.
- **Bindings:** An {ref}`application <application>` carries its default-binding space on its own record, and each charm endpoint can carry a space binding of its own; the `default-space` model configuration value names the space endpoints bind to by default.
- **Exposed endpoints:** {ref}`Exposed endpoints <the-application-operations>` grant spaces to the outside.
- **Constraints:** {ref}`Constraints <constraint>` can name the spaces a machine's subnets must come from.

(the-space-persistence-rules)=
## The space in the persistence layer

A space is persisted in the {ref}`model database <database>` as follows:

- **Space record:** The identity pair, a `uuid` primary key and a model-unique `name`; the unique index rejects a second space of the same name. The model seeds `alpha`, the default space.
  - *Related error:*
    - **`space not found`**: *Trigger:* A lookup by UUID or name that matches no space. *Remediation:* Verify the space exists in the model.
- **Provider space record:** The cloud's own identifier for the same grouping, where the cloud has one; at most one per space.
- **Subnet pointer:** The subnet carries the space pointer. The pointer is nullable, so a subnet belongs to at most one space or to none.
  - *Related error:*
    - **`subnet not found`**: *Trigger:* A lookup of a subnet, by CIDR or identifier, that matches no subnet. *Remediation:* Verify the subnet exists in the model.
- **Pointers that name the space:** An {ref}`application <application>` carries its default-binding space on its own record, each charm endpoint can carry a space binding of its own, and exposed endpoints grant spaces to the outside; {ref}`constraints <constraint>` can name the spaces a machine's subnets must come from.
  - *Rule:* Deleting a space resets what names it: constraints naming it are removed, default bindings and exposed endpoints move to `alpha`, endpoint bindings are dropped, and its subnets move to `alpha`.
- **States:** A space has no state machine and no life column: it is a naming record, created, renamed, or removed; nothing transitions.

## The space in the execution layer

A space has no machinery of its own: it is a grouping the controller maintains; subnets are moved into it and reloaded from the provider, and the network's watch surface reports what changed.

- **Container and VM network setup:** When a container or VM is provisioned, the network service matches its space requirements (the `spaces` constraint and the application's endpoint bindings) against the devices on its host.
  - *Related errors:*
    - **`space requirement conflict`**: *Trigger:* A space is both a positive and a negative space requirement for a machine (a constraint or endpoint binding naming it and a constraint excluding it). *Remediation:* Resolve the conflicting constraints or bindings.
    - **`space requirements unsatisfiable`**: *Trigger:* A container's or VM's space requirements cannot be met by its host (no device or bridge available in the required space). *Remediation:* Relax the space constraints or extend the host's networking in the required space.

(the-space-watchers)=
### Space watchers

The network domain exposes one watch surface:

- **Subnet changes:** The surfaces that need the network's shape (the provisioning and address machinery) watch subnets, not spaces; a space's change is implied by its subnets' moves.

Every watcher fires once immediately when it is created, the initial query being the baseline snapshot, and again on each qualifying change. See {ref}`the watcher pattern <watchers>`.

## Support for spaces in Juju providers

Support for spaces may vary from one cloud to another. For cloud-specific details, see the networking behavior section in each cloud's reference doc: {ref}`Amazon EC2 <cloud-ec2>`, {ref}`Microsoft Azure <cloud-azure>`, {ref}`OpenStack <cloud-openstack>`, {ref}`LXD <cloud-lxd>`, {ref}`MAAS <cloud-maas>`, {ref}`Unmanaged <cloud-unmanaged>`.
