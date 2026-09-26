---
myst:
  html_meta:
    description: "Juju storage reference: data volumes, storage directives, pools, providers, and dynamic storage management across clouds. The storage record, states, operations, watchers, and rules."
---

(storage)=
# Storage
```{audience} user
```

```{ibnote}
See also: {ref}`manage-storage`
```

In Juju, **storage** refers to a data volume that is provided by a {ref}`cloud <cloud>`.

Depending on how things are set up during deployment, the data volume can be machine-dependent (e.g., a directory on disk tied to the machine which goes away if the unit is destroyed) or machine-independent (i.e., it can outlive a machine and be reattached to another machine).

Most storage can be dynamically added to, and removed from, a unit. However, by their nature, some types of storage cannot be dynamically managed (see {ref}`storage-provider-maas`). Also, certain cloud providers may impose restrictions when attaching storage (see {ref}`storage-provider-ebs`).

(the-storages-records)=
## The storage's records

(the-storage-record)=
### The storage's identity

The persisted thing is the **storage instance**: one record per
provisioned piece of storage, carrying its name (the charm's storage
name plus an index), its kind (block or filesystem), its provision
scope (model -- machine-independent -- or machine -- dies with the
machine), and its life. The instance's backing is its own record --
the **volume** (block) or **filesystem** (filesystem) the cloud
provisioned -- and the **attachment** record binds the instance to a
{ref}`unit <unit>`. The storage instance's own record is created when
the charm's storage is requested, and the provisioned backing follows
once the cloud delivers it (see
{ref}`Storage operations <the-storage-operations>`).

(the-storage-in-the-data-model)=
### The storage in the data model

```{ggarch}
:file: ../juju.ggarch
:view: Storage model
:no-legend:
:caption: Topology: The storage walk in the model database: the charm defines storage names (kind block | filesystem, count, size); a directive pins one pool per application; an instance carries kind, life and requested size and is backed by exactly one volume or filesystem; attachments bind instances to units; volumes bind to net nodes. Provision scope model = machine-independent, machine = dies with the machine.
:alt: Record chain: charm storage, storage directive, storage pool, storage instance; volume to the right, filesystem below, attachment below charm storage, net node above volume.
```

The records: the {ref}`charm's <charm>` `charm_storage` definitions
(name, kind, count range, minimum size, whether the storage is shared
or read-only); the `storage_instance` records (the requested storage,
with kind, scope and life); their `storage_volume` /
`storage_filesystem` backings -- each bound to the machine's net node,
the same network identity the machine and its units share -- and the
`storage_attachment` records binding instances to units. Both backings
carry a transition-validated status record (see
{ref}`Storage states <the-storage-states>`); the volumes and
filesystems the provisioning machinery manages have their own state
machine on the provisioning side.

(the-storage-pool)=
(storage-pool)=
#### The storage pool

```{ibnote}
See also: {ref}`manage-storage-pools`
```

A **storage pool** is a mechanism for administrators to define sources of storage that they will use to satisfy application storage requirements.

A single pool might be used for storage from units of many different applications - it is a resource from which different stores may be drawn.

A pool describes {ref}`storage provider <storage-provider>`-specific parameters for creating storage, such as performance (e.g. IOPS), media type (e.g. magnetic vs. SSD), or durability.

For many providers, there will be a shared resource where storage can be requested (e.g. for Amazon EC2, `ebs`). Creating pools there maps provider specific settings into named resources that can be used during deployment.

Pools defined at the model level are easily reused across applications. Pool creation requires a pool name, the provider type and attributes for configuration as space-separated pairs, e.g. tags, size, path, etc.

In the data model, the pool is a first-class set of records: the pool
row (name, provider type), its attributes as key/value rows, its
origin (a user-created pool or a provider default), and the model's
per-kind **default pool** record -- created at model creation from the
provider's own defaults, and what an unnamed directive falls back to
(see {ref}`the directive's defaults <the-storage-directive-defaults>`).

(the-storage-states)=
### Storage states

A storage instance has no state machine of its own: its life is the
shared alive / dying / dead cycle, and the interesting state lives on
its **backing** -- the volume's or filesystem's status record, the one
status vocabulary in the model that is genuinely
**transition-validated**.

The vocabulary (identical for volumes and filesystems): `pending`,
`attaching`, `attached`, `detaching`, `detached`, `destroying`,
`error`, and `tombstone`. The transition law: a write is valid if the
status is unchanged; `tombstone` is terminal -- nothing moves out of
it; `pending` may only be (re)entered while the backing is not yet
provisioned (the retry case); every other transition is allowed.
What constrains storage is, as everywhere, who writes:

- **creation** inserts the backing as `pending`;
- the **storage provisioner** moves it through the provisioning path
  (`attaching`, `attached`) and records `error` on failures -- a
  transient error is retried, keeping the status where the retry
  resumes;
- **removal** sets `tombstone` once the provider has actually
  released the backing -- and a non-forced removal of the instance
  refuses until its backing reads dead *and* tombstoned.

(types-of-storage)=
### Types of storage

Storage carries two stored, exclusive discriminators: its **kind** --
`block` (a volume) or `filesystem` (a mounted filesystem), fixed by
the charm's definition -- and its **provision scope** -- `model`
(machine-independent: it can outlive its machine) or `machine` (it
dies with its machine). Every other distinction is the provider's
(see {ref}`storage providers <the-storage-operations>`).

(the-storages-machinery)=
## The storage's machinery

Storage has machinery of its own: in the controller, the storage
provisioner worker drives the instances' lifecycle -- provisioning
volumes and filesystems, writing their statuses, and seeing removals
through -- and the units' agents attach what it provisions.

(the-storage-operations)=
### Storage operations

(the-storage-directives)=
(storage-directive)=
#### Storage directives
In Juju, a **storage directive** is a collection of storage specifications that can be used to dictate how storage is allocated when provisioning storage for an application.

This directive has the form

```text
<name>[=<pool>, <count>, <size>>]
```

where
- `<name>` is the storage name as defined in the charm (see more: [Charmcraft | `<storage name>`](https://documentation.ubuntu.com/charmcraft/stable/reference/files/charmcraft-yaml-file/#storage));
- `<pool>` is a pre-defined {ref}`storage pool <storage-pool>`;
- `<count>` is the storage volume count;
- `<size>` is the size of each storage volume.

(the-storage-directive-defaults)=
The order of the arguments does not actually matter -- they are identified based on a regex (pool names must start with a letter and sizes must end with a unit suffix).

If at least one storage directive component is specified, the following default values come into effect:

* `<pool>`: the default storage pool.
* `<count>`: the minimum number required by the charm, or '1' if the storage is optional
* `<size>`: determined from the charm's minimum storage size, or 1GiB if the charm does not specify a minimum

In the absence of any explicit storage directive, the storage will be put on the root filesystem (`rootfs`).

Adding storage to a running unit (`juju add-storage`) or requesting it
at deploy time (`--storage`) writes the instance and its attachment
records with the backing's status at `pending`; the storage
provisioner takes it from there. The charm's own definitions merge
with the directive: the charm's count and minimum size are the floors
the user's overrides cannot go below.

#### Storage pool operations

Pools are managed as their own records: create, update (a full
replace), list (including the provider defaults), and delete. The
model's per-kind defaults are seeded at model creation from the
provider and re-written whenever the provider's recommendations
change; a directive that names no pool resolves to the model's
default for the storage kind.

#### Attaching storage

The attachment record is what binds an instance to a unit; on Juju
4.0 the detach operation is a stub (the service method is not yet
implemented on this branch) -- storage is released by removal instead
(see {ref}`storage removal <the-storage-removal>`).

(the-storage-removal)=
#### Storage removal

Removing storage (for example, `juju remove-storage`) is the
cooperative removal at its strictest: the instance is marked dying,
but the cascade refuses while attachment records remain (unless the
removal is forced); the removal job deletes the instance only once it
is not alive, and the backing's delete job only once the provisioner
has released the actual cloud resource and the backing reads dead and
tombstoned. The `--force` mode skips the gates.

(storage-provider)=
#### Storage providers
In Juju, a **storage provider** refers to the technology used to make storage available to a charm.

##### List of storage providers

There are three storage providers you can use with all clouds: `loop`, `rootfs`, and `tmpfs`. In addition, for some clouds there are also cloud-specific providers.

(storage-provider-cloud-specific)=
###### Cloud-specific storage providers

Many clouds provide additional storage providers beyond the generic ones. For the cloud-specific storage providers available on your cloud, see {ref}`list-of-supported-clouds` > `<cloud name>` > Storage.

###### `loop`
```{ibnote}
See also: [Wikipedia | Loop device](https://en.wikipedia.org/wiki/Loop_Device)
```

Block-type. Creates a file on the unit's root filesystem, associates a loop device with it. The loop device is provided to the charm.

```{note}
Loop devices require extra configuration to be used within LXD. See more: {ref}`storage-provider-lxd`.
```

###### `rootfs`
```{ibnote}
See also: [The Linux Kernel Archives | ramfs, rootfs and initramfs](https://www.kernel.org/doc/Documentation/filesystems/ramfs-rootfs-initramfs.txt)
```

Filesystem-type. Creates a sub-directory on the unit's root filesystem for the unit/charmed operator to use.

###### `tmpfs`
```{ibnote}
See also: [Wikipedia | Tmpfs](https://en.wikipedia.org/wiki/Tmpfs)
```

Filesystem-type. Creates a temporary file storage facility that appears as a mounted file system but is stored in volatile memory.

(the-storage-watchers)=
### Storage watchers
```{audience} juju-dev
```

The provisioning machinery exposes these watch surfaces -- what a
watcher fires on, not who consumes it (the storage provisioner worker
consumes them through the agents' facade):

- **Provisioned volumes** and **provisioned filesystems** -- the
  model-scoped and per-machine life changes of the backings; this is
  the provisioner's work queue.
- **Volume and filesystem attachments** -- model-scoped and
  per-machine, the attach/detach work.
- **Volume attachment plans** -- the pre-attach plans some providers
  need (iSCSI-style initiator setup).
- **Storage attachments** -- per attachment and per unit, the unit
  side of the story.

Every watcher fires once immediately when it is created -- the initial
query is the baseline snapshot -- and again on each qualifying change
(see {ref}`the watcher pattern <watchers>`).

(the-storage-rules-and-errors)=
## Storage rules and errors
```{audience} charm-dev
```

The rules a **storage directive** must satisfy:

- the components are identified by form, not order: the pool name
  starts with a letter, sizes carry a unit suffix (M/G/T/P), the
  count is a number;
- the charm's own floors hold: a directive cannot request fewer
  instances or a smaller size than the charm's definition requires.

The rules a **storage mutation** must satisfy:

- an instance cannot be removed while attachment records remain,
  unless the removal is forced (`storage instance still attached`);
- a backing's delete job requires it dead and tombstoned -- the
  provisioner's release is the gate (see
  {ref}`storage removal <the-storage-removal>`);
- the backing's status writes are transition-validated (the law under
  {ref}`Storage states <the-storage-states>`); an invalid transition
  is rejected (`volume status transition not valid` /
  `filesystem status transition not valid`).

(related-entities-storage)=
## Entities related to the storage

- **Charms** define the storage names, kinds, counts and minimum
  sizes the directives fill in (see {ref}`charm <charm>`).
- **Units** hold the attachments -- what the charm's hook context
  sees (see {ref}`unit <unit>`).
- Volumes and filesystems bind to **machines** through the shared net
  node (see {ref}`machine <machine>`).
- **Pools and providers** are where the backing comes from -- the
  pool names it, the provider creates it (see
  {ref}`the storage pool <the-storage-pool>`).
- **Constraints** steer the compute a root disk comes from -- the
  `root-disk-source` constraint names a pool (see
  {ref}`constraint <constraint>`).
- **Removal** owns the teardown, tombstone-gated (see
  {ref}`storage removal <the-storage-removal>`).
