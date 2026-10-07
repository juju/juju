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
(the-storage-declaration-rules)=
## Storage in the declaration layer

How clients request storage for applications and units, name the pools and providers that deliver it, and manage its life.

- **Storage directives:** A storage directive is a comma-separated sequence of pool, count, and size, set against a charm's storage name; the components are identified by form, not order.
  - *Rule:* A directive's count is a number; a size is a number with a unit multiplier (M through Y, powers of 1024).
- **Defaults:** A directive component left unspecified falls back: the pool to the model's default pool for the storage's kind, the count to 1, and the size to 1 GiB.
- **Removal intent:** Removing storage is the cooperative removal at its strictest: the instance refuses to go while attachment records remain, unless the removal is forced.
  - *Related error:*
    - **`storage instance still attached`**: *Trigger:* Removing a storage instance whose attachment records remain, without forcing. *Remediation:* Remove the attachments or the attached units first, or force the removal.
- **Detachment:** `juju detach-storage` removes the attachment between a storage instance and a unit, or every attachment of the instance when no unit is named; it can be forced and given a maximum wait.
  - *Rule:* Detaching is refused when it would take the unit below the charm's minimum number of storage instances for that storage name.

(the-storage-pools)=
(storage-pool)=
### Storage pools

A **storage pool** is a named set of storage-provider parameters: a pool name, the provider type, and the provider's attributes (for example tags, size, path) as pairs. A pool is a named, model-level source of storage: provider-specific settings (performance, media type, durability) mapped into a reusable resource that can serve many applications.

- **Pool operations:** Pools are managed as their own records: create, update (a full replace), list (including the provider defaults), and delete.
  - *Rule:* A pool name starts with a letter and continues with letters, digits, and hyphens.
  - *Related errors:*
    - **`storage pool already exists`**: *Trigger:* Creating a pool whose name is taken. *Remediation:* Use another name or update the pool.
    - **`storage pool name is invalid`**: *Trigger:* A pool name fails the pool-name grammar. *Remediation:* Start the name with a letter; use letters, digits, and hyphens.
    - **`storage pool is not found`**: *Trigger:* Querying or resolving a pool by a name that does not exist. *Remediation:* Check the model's pools, including the provider defaults.
    - **`provider type is invalid`**: *Trigger:* A pool names a provider type not valid for use in the model. *Remediation:* Use a provider type the model supports.
    - **`storage provider type not found`**: *Trigger:* A pool names a provider type the registry does not know. *Remediation:* Use a registered provider type.
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

(the-storage-persistence-rules)=
## Storage in the persistence layer

Storage is persisted in the {ref}`model database <database>` as follows:

- **Storage instance record:** A single primary entry containing the essential attributes: the name (the charm's storage name plus an index), the kind, the provision scope (model or machine), and its life.
  - *Related errors:*
    - **`storage instance not found`**: *Trigger:* Querying a storage instance that does not exist. *Remediation:* Verify the storage ID.
    - **`storage instance not alive`**: *Trigger:* Operating on a storage instance that is not alive. *Remediation:* None; the instance is being removed.
- **Charm storage definitions:** The {ref}`charm's <charm>` storage definitions carry the contract: the name, the kind (block or filesystem), the count range, the minimum size, and the shared and read-only flags.
- **Pool record:** A pool names a provider type and its origin: user-created or provider default, with the provider's parameters as key/value records. The model seeds a per-kind default pool at creation from the provider.
- **Directive record:** A resolved directive is pinned per application (or per unit, where the unit's charm temporarily diverges from its application's): the pool pointer, the size, and the count.
- **Backing records:** An instance has exactly one backing (a volume or a filesystem), bound to the machine's net node, the shared network identity the machine and its units anchor to; each carries its own status record.
  - *Rule:* The backing's status writes are transition-validated: a write is valid if the status is unchanged; `tombstone` is terminal, nothing moves out of it; `pending` may be re-entered only while the backing is not yet provisioned (the retry case); every other transition is allowed.
  - *Rule:* The vocabulary (identical for volumes and filesystems) is `pending`, `attaching`, `attached`, `detaching`, `detached`, `destroying`, `error`, `tombstone`.
  - *Rule:* A backing's delete job requires it dead and tombstoned: the provisioner's release of the cloud resource is the gate.
  - *Related errors:*
    - **`volume status transition not valid`**: *Trigger:* A status write moves a provisioned volume back into `pending` or writes over `tombstone`. *Remediation:* Let the provisioning machinery follow the transition path.
    - **`filesystem status transition not valid`**: *Trigger:* A status write moves a provisioned filesystem back into `pending` or writes over `tombstone`. *Remediation:* Let the provisioning machinery follow the transition path.
    - **`volume not found`**: *Trigger:* Reading the status of a volume ID that does not exist. *Remediation:* Verify the volume ID.
    - **`filesystem not found`**: *Trigger:* Reading the status of a filesystem ID that does not exist. *Remediation:* Verify the filesystem ID.
- **Attachment records:** An attachment binds the instance to its {ref}`unit <unit>`: the volume and filesystem attachment, and the attachment-plan satellites beneath it.
  - *Related error:*
    - **`storage attachment not found`**: *Trigger:* Querying a storage attachment that does not exist. *Remediation:* Verify the attachment.

The storage instance's own record is created when the charm's storage is requested; the provisioned backing follows once the cloud delivers it. The instance has no state machine of its own: its life is the shared alive, dying, dead cycle, and the interesting state lives on the backing's status record, the one status vocabulary in the model that is transition-validated.

**Writers:** The application storage service writes the instances, attachments, directives, and pools; the provisioning machinery writes the backings' statuses; the removal machinery carries the teardown.

## Storage in the execution layer

Storage has machinery of its own: in the controller, the storage provisioner worker drives the backings' lifecycle, provisioning volumes and filesystems, writing their statuses, and seeing removals through; the units' agents attach what it provisions.

(the-storage-watchers)=
### Storage watchers

The provisioning machinery exposes these watch surfaces:

- **Provisioned volumes and provisioned filesystems:** The model-scoped and per-machine life changes of the backings; the provisioner's work queue.
- **Volume and filesystem attachments:** Model-scoped and per-machine; the attach work.
- **Volume attachment plans:** The pre-attach plans some providers need (iSCSI-style initiator setup).
- **Storage attachments:** Per attachment and per unit, the unit side of the story.

Every watcher fires once immediately when it is created, the initial query being the baseline snapshot, and again on each qualifying change. See {ref}`the watcher pattern <watchers>`.
