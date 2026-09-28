---
myst:
  html_meta:
    description: "Juju storage reference: data volumes, storage directives, pools, providers, and dynamic storage management across clouds. The storage records, operations, watchers, and rules."
---

(storage)=
# Storage

```{ibnote}
See also: {ref}`manage-storage`
```

In Juju, **storage** is a data volume a {ref}`cloud <cloud>` provides to a {ref}`unit <unit>`: machine-dependent, dying with its machine, or machine-independent, able to outlive its machine and reattach to another one.

(storage-directive)=
## The storage in the declaration layer

How clients request storage for applications and units, name the pools and providers that deliver it, and manage its life.

- **Storage directives:** A storage directive is a comma-separated sequence of pool, count, and size, set against a charm's storage name; the components are identified by form, not order.
- **Defaults:** A directive component left unspecified falls back: the pool to the model's default pool for the storage's kind, the count to 1, and the size to 1 GiB.
- **Pools:** A pool is a named, model-level source of storage: provider-specific settings (performance, media type, durability) mapped into a reusable resource that can serve many applications.
- **Removal intent:** Removing storage is the cooperative removal at its strictest: the instance refuses to go while attachment records remain, unless the removal is forced.
- **Detachment:** Detaching storage from its unit is not implemented on Juju 4.0; storage is released through removal instead.

(the-storage-pools)=
(storage-pool)=
### Storage pools

A **storage pool** is a named set of storage-provider parameters: a pool name, the provider type, and the provider's attributes (for example tags, size, path) as pairs.

- **Pool operations:** Pools are managed as their own records: create, update (a full replace), list (including the provider defaults), and delete.
- **Default pools:** The model's per-kind default pools are seeded at model creation from the provider's own recommendations; a directive that names no pool resolves to the model's default for the storage kind.

```{ibnote}
See also: {ref}`manage-storage-pools`
```

(storage-provider-cloud-specific)=
(storage-provider)=
### Storage providers

A **storage provider** is the technology used to make storage available to a charm.

- **Everywhere:** Three providers work with every cloud: `loop` (block; a file on the unit's root filesystem with an associated loop device provided to the charm), `rootfs` (filesystem; a subdirectory on the unit's root filesystem), and `tmpfs` (filesystem; a memory-backed mounted filesystem).
- **Cloud-specific:** Many clouds provide additional providers beyond the generic ones. For the cloud-specific providers available on your cloud, see {ref}`list-of-supported-clouds` > `<cloud name>` > Storage.
- **Provider restrictions:** Some providers cannot be dynamically managed or impose restrictions when attaching storage; see each cloud's provider section (for example {ref}`storage-provider-maas`, {ref}`storage-provider-ebs`).

(the-storage-declaration-rules)=
### Declaration rules and errors

- **Rules:**
  - A pool name starts with a letter and continues with letters, digits, and hyphens; a directive's count is a number; a size is a number with a unit multiplier (M through Y, powers of 1024).
- **Errors:**
  - **`storage instance still attached`:** Triggered when removing a storage instance whose attachment records remain, without forcing. Remediation: remove the attachments or the attached units first, or force the removal.
  - **`storage pool already exists`:** Triggered when creating a pool whose name is taken. Remediation: use another name or update the pool.
  - **`storage pool name is invalid`:** Triggered when a pool name fails the pool-name grammar. Remediation: start the name with a letter; use letters, digits, and hyphens.
  - **`storage pool is not found`:** Triggered when querying or resolving a pool by name that does not exist. Remediation: check the model's pools, including the provider defaults.
  - **`provider type is invalid`:** Triggered when a pool names a provider type not valid for use in the model. Remediation: use a provider type the model supports.
  - **`storage provider type not found`:** Triggered when a pool names a provider type the registry does not know. Remediation: use a registered provider type.

## The storage in the persistence layer

```{ggarch}
:file: ../juju.ggarch
:view: Storage model
:no-legend:
:caption: Topology: The storage walk in the model database: the charm defines storage names (kind block | filesystem, count, size); a directive pins one pool per application; an instance carries kind, life and requested size and is backed by exactly one volume or filesystem; attachments bind instances to units; volumes bind to net nodes. Provision scope model = machine-independent, machine = dies with the machine.
:alt: Record chain: charm storage, storage directive, storage pool, storage instance; volume to the right, filesystem below, attachment below charm storage, net node above volume.
```

In the model database, the persisted thing is the **storage instance**; its records are (`0011-storage.sql`):

- **`charm_storage`**: The {ref}`charm's <charm>` storage definitions: the name, the kind (block or filesystem), the count range, the minimum size, and the shared and read-only flags.
- **`storage_pool`, `storage_pool_attribute`, `storage_pool_origin`**: The pool record naming its provider type, the provider's parameters as key/value rows, and the pool's origin (user-created or provider default).
- **`model_storage_pool`**: The model's per-kind default pool record, seeded at model creation from the provider.
- **`application_storage_directive`, `unit_storage_directive`**: The resolved directive pinned per application (or per unit, where the unit's charm temporarily diverges from its application's): the pool pointer, the size, and the count.
- **`storage_instance`**: One record per provisioned piece of storage: the name (the charm's storage name plus an index), the kind, the provision scope (model or machine), and its life.
- **`storage_volume`, `storage_filesystem`**: The instance's backing, exactly one per instance, bound to the machine's net node, the shared network identity the machine and its units anchor to; each carries its own status record.
- **`storage_attachment`**: The record binding the instance to a {ref}`unit <unit>`, with the volume and filesystem attachment and attachment-plan satellites beneath it.

The storage instance's own record is created when the charm's storage is requested; the provisioned backing follows once the cloud delivers it. The instance has no state machine of its own: its life is the shared alive, dying, dead cycle, and the interesting state lives on the backing's status record, the one status vocabulary in the model that is transition-validated.

(the-storage-persistence-rules)=
### Persistence rules and errors

- **Rules:**
  - The backing's status writes are transition-validated: a write is valid if the status is unchanged; `tombstone` is terminal, nothing moves out of it; `pending` may be re-entered only while the backing is not yet provisioned (the retry case); every other transition is allowed.
  - The vocabulary (identical for volumes and filesystems) is `pending`, `attaching`, `attached`, `detaching`, `detached`, `destroying`, `error`, `tombstone`.
  - A backing's delete job requires it dead and tombstoned: the provisioner's release of the cloud resource is the gate.
- **Errors:**
  - **`volume status transition not valid`:** Triggered when a status write moves a provisioned volume back into `pending` or writes over `tombstone`. Remediation: let the provisioning machinery follow the transition path.
  - **`filesystem status transition not valid`:** Triggered when a status write moves a provisioned filesystem back into `pending` or writes over `tombstone`. Remediation: let the provisioning machinery follow the transition path.
  - **`storage instance not found`:** Triggered when querying a storage instance that does not exist. Remediation: verify the storage ID.
  - **`storage instance not alive`:** Triggered when operating on a storage instance that is not alive. Remediation: none; the instance is being removed.
  - **`storage attachment not found`:** Triggered when querying a storage attachment that does not exist. Remediation: verify the attachment.
  - **`volume not found`:** Triggered when reading the status of a volume ID that does not exist. Remediation: verify the volume ID.
  - **`filesystem not found`:** Triggered when reading the status of a filesystem ID that does not exist. Remediation: verify the filesystem ID.

Writers: the application storage service writes the instances, attachments, directives, and pools; the provisioning machinery writes the backings' statuses; the removal machinery carries the teardown.

## The storage in the execution layer

Storage has machinery of its own: in the controller, the storage provisioner worker drives the backings' lifecycle, provisioning volumes and filesystems, writing their statuses, and seeing removals through; the units' agents attach what it provisions.

(the-storage-watchers)=
### Storage watchers

The provisioning machinery exposes these watch surfaces:

- **Provisioned volumes and provisioned filesystems:** The model-scoped and per-machine life changes of the backings; the provisioner's work queue.
- **Volume and filesystem attachments:** Model-scoped and per-machine; the attach work.
- **Volume attachment plans:** The pre-attach plans some providers need (iSCSI-style initiator setup).
- **Storage attachments:** Per attachment and per unit, the unit side of the story.

Every watcher fires once immediately when it is created, the initial query being the baseline snapshot, and again on each qualifying change. See {ref}`the watcher pattern <watchers>`.