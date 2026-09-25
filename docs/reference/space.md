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

A Juju **(network) space** is a logical grouping of {ref}`subnets <subnet>` that can communicate with one another.

A space is used to help segment network traffic for the purpose of:
* Network performance
* Security
* Controlling the scope of regulatory compliance

(the-spaces-records)=
## The space's records

(the-space-record)=
### The space's identity

In the model database, a space is a record: its name and UUID. Two
satellites complete it: the **provider space** record (the cloud's
own identifier for the same grouping, where the provider has one) and
the space's subnets -- pointed *at* the space: each
{ref}`subnet <subnet>` row carries the space it belongs to, so a
subnet belongs to exactly one space or to none.

(the-space-in-the-data-model)=
### The space in the data model

```{ggarch}
:file: ../juju.ggarch
:view: Network spaces
:no-legend:
:caption: Topology: A space groups subnets; a subnet belongs to 0..1 space (the alpha space exists by default); an application's default binding points at one space, and each charm-relation endpoint can bind 0..1 space of its own.
:alt: Application record to space record to subnet record.
```

The space's reach beyond its own records is through the pointers that
name it: the model's `default-space` configuration value names the
space endpoints bind to by default; an
{ref}`application <application>` carries its default-binding space on
its own record, and each charm endpoint can carry a space binding of
its own; {ref}`exposed endpoints <the-application-operations>` grant
spaces to the outside; and {ref}`constraints <constraint>` can name
the spaces a machine's subnets must come from.

(the-space-states)=
### Space states

A space has no state machine and no life column: it is a naming
record, created, renamed, or removed -- nothing transitions.

(the-spaces-machinery)=
## The space's machinery

A space has no machinery of its own: it is a grouping the controller
maintains -- subnets are moved into it and reloaded from the provider,
and the one watch surface (subnet changes) reports what changed.

(the-space-operations)=
### Space operations

#### Adding, renaming and removing a space

Adding a space (`juju add-space`) creates the record; renaming rewrites
the name; removing (with a dry-run mode that reports what would break)
deletes it -- subnets that pointed at it return to no space.

#### Moving subnets

Subnets are moved between spaces (for example, `juju move-subnet`):
the move rewrites the subnet's space pointer.

#### Reloading spaces from the provider

The provider can be asked to rediscover its spaces and subnets: where
the cloud exposes its own space notion, the reload adopts it; where it
does not, everything falls into the default `alpha` space.

(the-space-watchers)=
### Space watchers

The network domain exposes one watch surface: **subnet changes** --
the surfaces that need the network's shape (the provisioning and
address machinery) watch subnets, not spaces; a space's change is
implied by its subnets' moves.

Every watcher fires once immediately when it is created -- the
initial query is the baseline snapshot -- and again on each qualifying
change: database triggers feed the change stream, the watcher wakes,
and the consumer fetches the current state and reconciles.

(the-space-rules-and-errors)=
## Space rules and errors

The rules a **space name** must satisfy:

- a valid space name -- lowercase letters, digits and hyphens (the
  same name grammar as applications); `alpha` is the default space
  every model starts with.

The rules a **space mutation** must satisfy:

- a space name is unique per model (`space already exists`);
- removing or re-homing a space can violate the model's space
  requirements -- constraints and bindings that name it -- and the
  removal machinery reports those violations (dry-run first), failing
  with `space requirement conflict` / `space requirements
  unsatisfiable` when they cannot be met;
- a subnet belongs to at most one space.

The errors that encode them: `space already exists`, `space not
found`, `subnet not found`, `space name not valid`, `space requirement
conflict`.

(the-space-rules-bindings)=
### Spaces as constraints and bindings

Spaces can be specified as {ref}`constraints <constraint>` -- to determine what subnets a machine is connected to -- or as application endpoint bindings -- to determine the subnets used by application relations.

A binding associates an {ref}`application endpoint <application-endpoint>` with a space. This restricts traffic for the endpoint to the subnets in the space. By default, endpoints are bound to the space specified in the `default-space` model configuration value. The name of the default space is "alpha".

Constraints and bindings affect application deployment and machine provisioning as well as the subnets a machine can talk to.

Endpoint bindings can be specified during deployment with `juju deploy --bind` or changed after deployment using the `juju bind` command.

(the-space-providers)=
## Support for spaces in Juju providers

Support for spaces may vary from one cloud to another. For cloud-specific details, see the networking behavior section in each cloud's reference doc: {ref}`Amazon EC2 <cloud-ec2>`, {ref}`Microsoft Azure <cloud-azure>`, {ref}`OpenStack <cloud-openstack>`, {ref}`LXD <cloud-lxd>`, {ref}`MAAS <cloud-maas>`, {ref}`Unmanaged <cloud-unmanaged>`.

(related-entities-space)=
## Entities related to the space

- **Subnets** are what a space groups; the membership pointer lives
  on the subnet (see {ref}`subnet <subnet>`).
- **Applications and their endpoints** bind to spaces -- the default
  binding and the per-endpoint bindings (see
  {ref}`application <application>`,
  {ref}`application endpoint <application-endpoint>`).
- **Constraints** name spaces to steer where machines land
  (see {ref}`constraint <constraint>`).
- **Machines** get their addresses from the subnets the bindings
  select (see {ref}`machine <machine>`).
