---
myst:
  html_meta:
    description: "Juju storage reference: what charm storage is, the records the model keeps about directives, pools, instances, volumes, filesystems, and attachments, the changes that reach units and machines, and the calls that add, list, attach, detach, remove, and provision storage."
---

(storage)=
# Storage

```{ibnote}
See also: {ref}`Juju | Manage storage <manage-storage>`, {ref}`Juju | Manage storage pools <manage-storage-pools>`
```

A **storage** is a named store that a {ref}`charm <charm>` declares (such as a database directory or a block device) and that Juju provisions for a {ref}`unit <unit>` from a {ref}`storage pool <storage-pool>` of the {ref}`model <model>`. A filesystem storage lives on the machine that hosts the unit and dies with it. A block storage is a volume that can outlive that machine and attach to another one. A charm can ask for more of its storage while the unit runs, and can read what it got, with the {ref}`storage-add <hook-command-storage-add>` and {ref}`storage-get <hook-command-storage-get>` hook commands.

(storage-database-schema)=
## Storage database schema

```{ggarch}
:file: ../juju.ggarch
:view: Storage database schema
:caption: The model database keeps one record for each named store. A storage directive belongs to one application or one unit, names one charm storage, and draws from one pool. A model names a default pool for each kind of storage, and the kind is one of two: block or filesystem. A pool has any number of attributes, and any number of directives and storages draw from it. A storage belongs to one unit through its owner record, and the units that the charm allows attach it through attachment records (a unit appears once per storage). A storage has at most one volume and at most one filesystem, and each of those links back to at most one storage. A volume attaches to one network node (a machine or a container) at a time, and names the block device it brought on that node. An attachment plan is the planned form of a volume attachment: one per volume and node, naming the device type before the volume is on the machine. The application, the unit, the charm storage, the network node, and the block device are described on their own pages, and the figure shows them only for context. All boxes are records, and the five with a second, outlined arrow mark in the corner are the records that other pages describe.
:alt: Twenty-one boxes of the same size inside a group named Model database, each with a filled table mark in the corner. Application is in the top left, with Unit below it and Charm storage below Unit. Application directive is to the right of Application, and Unit directive to the right of Unit; both directives join down and to the left to Charm storage. Storage pool is in the middle, with Pool attribute above it, Model default pool below it, and Kind below that. Storage instance is to the right of the pool, with Unit owner above it and Storage attachment below it; Unit owner and Storage attachment both reach Unit on the far left. Instance volume is to the right of Unit owner, then Volume, then Volume attachment, with Block device above Volume attachment and Network node on the far right. Instance filesystem is to the right of Storage attachment, then Filesystem, then Filesystem attachment, which also reaches Network node. Attachment plan sits below Volume attachment and reaches both Volume and Network node. Application, Unit, Charm storage, Network node, and Block device have an outlined arrow mark for records that other pages describe.
```

The storage database schema is the set of records that the {ref}`model database <database>` keeps about charm storage. The records, with the rules that apply to each, are grouped by who owns a row and when it dies:

(storage-directive)=
### Storage directives

A directive tells Juju what to create for one storage name of a charm. The records are:

- **Application storage directive:** What an application's charm asks for, for one storage name: the pool, the count, and the size. The rows of an application are replaced when its charm changes.

  A client writes a directive as `<name>[=<pool>, <count>, <size>]`, where `<name>` is a storage name of the charm, `<pool>` a pool name, `<count>` a number of stores, and `<size>` a size such as `3G` or `100M`. The order of the three parts does not matter; each is told apart by its shape. A directive of `<name>` alone asks for one store. A part that a client leaves out is filled in when Juju records the directive: the pool from the model's default pool for the storage's kind, the count from the charm's minimum, and the size from the charm's minimum, or 1 GiB when the charm sets none.
  - *Rules:*
    - An application has each storage name once for each of its charms.
    - The application, the pool, and the charm's storage all exist.
- **Unit storage directive:** The same record for one unit. A unit's charm can differ from its application's for a while, so a unit's directives are kept apart from the application's.
  - *Rules:*
    - A unit has each storage name once for each of its charms.
    - The unit, the pool, and the charm's storage all exist.
- **Application, unit, and charm storage:** The records at the far ends of a directive. Other pages own them: see {ref}`application <application>`, {ref}`unit <unit>`, and {ref}`charm <charm>`.

(storage-pool)=
### Storage pools
A pool is a named source of storage in the model. The records, with the rules that apply to each, are:

- **Storage pool:** A named source of storage that an administrator defines or that a provider offers: its name, its type, and where it came from. The type names a {ref}`storage provider <storage-provider>`.

  (storage-provider)=
  A **storage provider** is the technology that makes storage available to a charm. Three providers work with every cloud: `rootfs`, which creates a directory on the unit's root filesystem; `loop`, which creates a file there and attaches it as a loop device; and `tmpfs`, which creates a filesystem in volatile memory. A model on a cloud can also use the providers of that cloud, such as `ebs` on Amazon EC2 or `cinder` on OpenStack.

  (storage-provider-cloud-specific)=
  The providers that a cloud offers are listed with the cloud: see {ref}`list-of-supported-clouds` and, under each cloud, its Storage section. Some providers cannot do everything the generic ones can: the MAAS provider, for one, provisions no volume of its own, so it only tags the root disks of the machines it creates.
  - *Rules:*
    - A pool's name appears once in the model.
    - A name and a type are never empty.
    - The origin is one of `user` (an administrator created it) and `provider-default` (the model was created with it).
    - The origin of a pool cannot change.
- **Pool attribute:** One key and value of a pool, such as the volume type of an EBS pool or the path of a `rootfs` pool.
  - *Rules:*
    - A pool has each key once.
- **Model default pool:** The pool that a storage of a given kind draws from when its directive names no pool. One row names the kind and the pool. The model config keys `storage-default-block-source` and `storage-default-filesystem-source` choose them.
- **Kind:** What a storage is: one of `block` (a volume) and `filesystem` (a mount). The charm fixes the kind when it declares the storage.

### Storage instances

A unit owns the storages that its charm asked for. The records are:

- **Storage instance:** The record that a charm's named storage gets: its storage ID in the form `<name>/<number>`, its name, its kind, its life (alive, dying, or dead), its pool, and the size that was asked for. Juju deletes a dying storage and never marks it dead. A storage that was created but never attached names no charm. The first attachment names the charm; the storage never serves another one.
  - *Rules:*
    - A storage ID appears once in the model.
    - The pool exists.
- **Unit owner:** The unit that owns a storage. When the unit is removed, the storage dies with it. This is not an attachment: an owner holds the storage for its unit, and other units can still attach it while it lives, when the charm allows.
  - *Rules:*
    - A storage has at most one owning unit.
- **Storage attachment:** A unit that a storage is attached to, with the attachment's life.
  - *Rules:*
    - A unit appears once per storage.
- **Application and unit:** The records that own or attach storages. Other pages describe them: see {ref}`application <application>` and {ref}`unit <unit>`.

### Volumes and filesystems

A volume and a filesystem are one design twice: the first is a block device, the second a mount. Each record below exists for both kinds, with the same rules:

- **Volume and filesystem:** What backs a storage: its own ID, its life (alive, dying, or dead), its provision scope (`model` or `machine`), the provider ID under which the provider knows it, its size, and whether to obliterate it — destroy it on the cloud — when it is cleaned up. A volume also carries its hardware ID and worldwide name, and whether it is persistent, so that it can outlive its machine. A filesystem is created by a provider (as EBS plus a mount) or by a charm's `rootfs` pool, which makes a directory.
  - *Rules:*
    - A volume or filesystem ID appears once in the model.
    - A provider ID of a filesystem appears once.
    - The obliterate mark is set only while the volume or filesystem is not alive.
- **Instance volume and instance filesystem:** The link between a storage and what backs it.
  - *Rules:*
    - A storage links to at most one volume and at most one filesystem.
    - A volume or filesystem links back to at most one storage.
- **Volume status and filesystem status:** The status of what backs a storage, with a message and the time of the last change: one of `pending`, `error`, `attaching`, `attached`, `detaching`, `detached`, `destroying`, and `tombstone` (an internal status for a dead record that is all but deleted).
  - *Rules:*
    - A volume has at most one status. A filesystem has at most one status.

### Attachments to machines

An attachment puts a volume or a filesystem on the network node of a machine or a container. The records are:

- **Volume attachment:** A volume attached to a network node (a machine or a container), with the attachment's life, its provision scope, the provider ID of the attachment, whether it is read-only, and the block device it brought on that node.
  - *Rules:*
    - A volume attaches to at most one node.
    - The volume, the node, and the block device all exist.
- **Filesystem attachment:** A filesystem attached to a network node, with its mount point, whether it is read-only, its life, its provision scope, and the provider ID of the attachment.
  - *Rules:*
    - The filesystem and the node exist.
- **Attachment plan:** The planned form of a volume attachment: what Juju must do to make the volume appear on the node, before the volume is there. The plan names the device type, one of `local` (a disk on the machine) and `iscsi` (a device reached over the network), plus free-form attributes for the provider, such as an iSCSI target.
  - *Rules:*
    - A volume has at most one plan per node.
- **Plan attribute:** One key and value of a plan.
  - *Rules:*
    - A plan has each key once.
- **Machine volume and machine filesystem:** The machines that host a volume or a filesystem, so that the units on those machines can find them.
  - *Rules:*
    - A machine hosts a volume once. A machine hosts a filesystem once.
- **Machine and block device:** The records that host and carry storages. Other pages describe them: see {ref}`machine <machine>`. A block device is a disk that a machine's agent reported, with the names under which the machine links to it; the {ref}`machine <machine>` page describes it.

(storage-change-streams)=
## Storage change streams

Storage change streams are the notifications that tell the agents of a model what happened to its storage, so that each agent acts when its part is due. A change stream is delivered through a watcher (an API object that tells its holder when something it follows has changed). The streams carry:

- **A unit's attachments:** A unit agent calls `WatchUnitStorageAttachments` to hear that a storage of its unit enters, changes, or leaves, and `WatchStorageAttachments` to hear that one attachment's details change. The agent runs the {ref}`storage-attached <hook-storage-attached>` hook for a newly attached storage. It runs the {ref}`storage-detaching <hook-storage-detaching>` hook for one that is being taken away.
- **Block devices:** A machine agent calls `WatchBlockDevices` to hear that the disks of one of its machines changed, so that it can match them to the volumes it attaches.
- **Volumes and filesystems:** A machine agent calls `WatchVolumes` and `WatchFilesystems` to hear that a volume or a filesystem was added to the model or changed, scoped to the whole model or to one machine.
- **Attachments:** A machine agent calls `WatchVolumeAttachments` and `WatchFilesystemAttachments`, or the `VolumeAttachmentsWatcher` and `FilesystemAttachmentsWatcher` facades, to hear that a volume or a filesystem of the model is to be attached to or detached from one of its machines.
- **Plans:** A machine agent calls `WatchVolumeAttachmentPlans`, or the `VolumeAttachmentPlansWatcher` facade, to hear that a plan exists for attaching a volume to one of its machines.

No stream reports storage pools: a pool changes only when a client changes it.

(storage-calls)=
## Storage calls

Storage calls are the calls on the storages, pools, volumes, filesystems, and attachments of a model. A {ref}`client <client>` takes them through the controller API (the set of calls that clients and {ref}`agents <agent>` make to the controller). A **unit agent** is the {ref}`agent <agent>` process that runs a unit's charm: it logs in as its unit and acts only for that unit. A **machine agent** is the agent process of a machine: it acts for its own machine and for the containers on it. A controller's machine agent acts for the whole model. The facades that serve the agents accept no model access level, so none of the agent calls below name one. Clients act first, in the order of the {ref}`storage how-to <manage-storage>`: add, view, detach, attach, import, remove; then the pool how-to adds and lists pools.

(storage-listing)=
### Listing storage

```{ibnote}
See also: {ref}`Juju | View the available storage <view-the-available-storage>`, {ref}`Juju | View storage details <view-storage-details>`
```

Listing storage returns the storages of the model. The answer carries the volumes and filesystems that back them, with their statuses, owners, attachments, machines, and locations.

The caller, the rules, and the errors are:

- *Caller:* A client, for a user with {ref}`read access <user-access-model-read>` to the model, or with {ref}`superuser access <user-access-controller-superuser>` to the controller. No block applies.
- *Rules:*
  - `ListStorageDetails` returns every storage of the model: its tag, its owner unit, its kind, its life, whether it is persistent, its status, and its attachments with their machines and locations. The call takes at least one filter, and returns the same list for each filter; filters do not select anything.
  - `StorageDetails` returns the details of the storages the caller names by tag, one result per tag. A client shows one storage with `juju storage show`.
  - `ListVolumes` and `ListFilesystems` take the same filters and return what backs the storages: the volume or filesystem, its provider ID, its size, its life, its status, the storage it belongs to, its attachments with their machines and devices or mount points, and the machine that hosts it.
  - A machine filter narrows a volume or filesystem list to the ones attached to that machine.
- *Errors:* The client receives each as the error of the whole call, except the last, which arrives in the result for the tag:
  - **`not authorized for request`**: *Trigger:* The caller has neither read access to the model nor superuser access. The client receives it with the code `unauthorized access`. *Remediation:* Ask a model administrator.
  - **`at least one filter is required`**: *Trigger:* The call gave no filter. The client receives it with the code `not valid`. *Remediation:* Give one filter.
  - **`invalid storage entity tag %d`** (code `not valid`): *Trigger:* A tag in the `StorageDetails` request is malformed. *Remediation:* Use the tag of a storage of the model.
  - **`tag kind %q not supported`** (code `not valid`): *Trigger:* A tag in the `StorageDetails` request is not a storage tag. *Remediation:* Name a storage by its tag.
  - **`invalid storage id supplied`** (code `not valid`): *Trigger:* The ID inside a storage tag is not a valid storage ID. *Remediation:* Use the tag of a storage of the model.
  - **`storage instance %q not found`** (code `not found`): *Trigger:* No storage has the ID that the tag names. *Remediation:* {ref}`List the storages <storage-listing>` and use one of them.

(storage-pool-listing)=
### Listing storage pools

```{ibnote}
See also: {ref}`Juju | Manage storage pools <manage-storage-pools>`, {ref}`Terraform Provider for Juju | Manage storage pools <tfjuju:manage-storage-pools>`
```

Listing storage pools returns the pools of the model, with the provider type of each and its attributes.

The caller, the rules, and the errors are:

- *Caller:* A client, for a user with {ref}`read access <user-access-model-read>` to the model, or with {ref}`superuser access <user-access-controller-superuser>` to the controller. No block applies.
- *Rules:*
  - Each filter returns the pools that match it: by name, by provider, or by both at once.
  - A list without filters returns every pool of the model.
  - Each result carries the pool's name, its provider, and its attributes.
- *Errors:* **`not authorized for request`** (code `unauthorized access`): *Trigger:* The caller has neither read access nor superuser access. *Remediation:* Ask a model administrator.

(storage-pool-creating)=
### Creating storage pools

```{ibnote}
See also: {ref}`Juju | Manage storage pools <manage-storage-pools>`, {ref}`Terraform Provider for Juju | Manage storage pools <tfjuju:manage-storage-pools>`
```

Creating storage pools stores a new pool in the model: a name, a provider type, and attributes for that provider, such as the volume type of an EBS pool or the path of a `rootfs` pool.

The caller, the rules, and the errors are:

- *Caller:* A client, for a user with {ref}`write access <user-access-model-write>` to the model. No block applies.
- *Rules:*
  - The name follows RFC 1035, with up to 128 characters: it starts with a letter, continues with letters, digits, and single hyphens, and ends with a letter or a digit.
  - The provider must be one that the model's cloud offers.
  - The call takes a list of pools. Each pool in the list gets its own result.
- *Errors:* The client receives each in the result for the pool:
  - **`storage pool name is either not set or is not a valid name`** (code `not valid`): *Trigger:* The name breaks the rule above. *Remediation:* Choose a name that follows it.
  - **`provider type is not a valid value that can be used for a new storage pool`** (code `not valid`): *Trigger:* The provider type is not a valid value. *Remediation:* Name one of the model's providers.
  - **`no storage provider %q exists in the current model`** (code `not found`): *Trigger:* The model's cloud offers no such provider. *Remediation:* {ref}`List the pools <storage-pool-listing>` and use one of their providers.
  - **`storage pool for name %q already exists in the model`** (code `already exists`): *Trigger:* Another pool has the name. *Remediation:* Choose another name, or {ref}`update the existing pool <manage-storage-pools>`.
  - **`storage pool attribute %q is not valid: <reason>`** (code `not valid`): *Trigger:* A value that the provider rejects for that key. *Remediation:* Give the value that the reason asks for.

### Updating and removing storage pools

A client cannot yet update or remove a pool: both calls answer `not yet available in <version>` with the code `not yet available; try again later`, whatever the arguments say.

(storage-adding)=
### Adding storage to units

```{ibnote}
See also: {ref}`Juju | Add storage <add-storage>`, {ref}`Ops | Manage storage <ops:manage-storage>`
```

Adding storage to a unit creates new storages of one of the unit's charm storage names, from the directive that the unit already keeps. A client adds storages with `juju add-storage`. A charm adds them with the {ref}`storage-add <hook-command-storage-add>` hook command, which takes a storage name and a count, and takes effect when the hook that ran it succeeds.

The caller, the rules, and the errors are:

- *Caller:* A client, for a user with {ref}`write access <user-access-model-write>` to the model. The change block (see {ref}`restricting commands on a model <restrict-commands-on-a-model>`) must not be on. A charm's `storage-add` reaches the same call through its unit agent.
- *Rules:*
  - The request names one unit and one storage name, and may override the pool and the size of the unit's directive, and the count.
  - The unit's charm must declare the storage name. The unit must be alive.
  - The count, with the storages the unit already has, must stay inside the charm's minimum and maximum for that name.
  - The pool must exist.
  - The call answers with the tags of the storages it created, in the form `storage-<name>-<number>`.
  - On a Kubernetes model the storages become persistent volume claims. On a machine model they become volumes or filesystems on the unit's machine.
- *Errors:* The client receives each in the result for the request:
  - **`not authorized for request`** (code `unauthorized access`): *Trigger:* The caller lacks write access to the model. This fails the whole call. *Remediation:* Ask a model administrator.
  - **the operator's block message, or `the operation has been blocked` when the operator gave none** (code `operation is blocked`): *Trigger:* The change block is on. This fails the whole call. *Remediation:* Ask the operator to lift the block.
  - **`"<tag>" is not a valid unit tag`**: *Trigger:* The unit tag is malformed. The client receives it with no error code. *Remediation:* Use the tag of a unit of the model.
  - **`invalid unit name %q`** (code `not valid`) and **`unit %q does not exist`** (code `not found`): *Trigger:* The unit name inside the tag is not a valid unit name, or names no unit. *Remediation:* Name a unit of the model.
  - **`invalid storage pool name`** (code `not valid`) and **`storage pool %q does not exist`** (code `not found`): *Trigger:* The pool override is not a valid pool name, or names no pool. *Remediation:* {ref}`List the pools <storage-pool-listing>` and use one of them.
  - **`storage directive %s count %d too large`** (code `not valid`): *Trigger:* The count does not fit in the request. *Remediation:* Ask for fewer storages.
  - **`invalid storage name %q`** (code `not valid`) and **`storage name %q not supported by charm`** (code `not supported`): *Trigger:* The storage name is malformed, or the unit's charm declares no such storage. *Remediation:* Use a storage name of the charm.
  - **`storage directive %q request count %d exceeds the charm's maximum count of %d`** (code `not valid`): *Trigger:* The count with the storages already attached passes the charm's maximum. *Remediation:* Remove a storage first, or ask for fewer.

(storage-attaching)=
### Attaching storages to units

```{ibnote}
See also: {ref}`Juju | Attach storage <attach-storage>`, {ref}`Juju | Reuse storage <reuse-storage>`
```

Attaching a storage to a unit hands an existing storage to a unit that the charm allows to use it. A client attaches a detached storage with `juju attach-storage`, and can deploy or add a unit with `--attach-storage`.

The caller, the rules, and the errors are:

- *Caller:* A client, for a user with {ref}`write access <user-access-model-write>` to the model. The change block (see {ref}`restricting commands on a model <restrict-commands-on-a-model>`) must not be on.
- *Rules:*
  - The unit and the storage must both exist and be alive.
  - The unit's charm must declare the storage's name. The storage's kind must fit that declaration.
  - The storage must have been made for the charm of the unit. Its size must meet the charm's minimum.
  - The number of the unit's storages of that name must stay inside the charm's maximum.
  - The storage must not be attached to the unit already. A storage that has attachments is attached again only when the charm's storage is shared.
  - A unit on a machine takes a machine-scoped volume that is bound to that unit's machine.
- *Errors:* The client receives each in the result for the request, except the first two, which fail the whole call:
  - **`not authorized for request`** (code `unauthorized access`): *Trigger:* The caller lacks write access to the model. *Remediation:* Ask a model administrator.
  - **the operator's block message, or `the operation has been blocked` when the operator gave none** (code `operation is blocked`): *Trigger:* The change block is on. *Remediation:* Ask the operator to lift the block.
  - **`invalid unit tag`** and **`invalid unit name %q`** (code `not valid`): *Trigger:* The unit tag is malformed. *Remediation:* Name a unit of the model.
  - **`unit %q does not exist`** (code `not found`): *Trigger:* The unit names no unit. *Remediation:* Name a unit of the model.
  - **`invalid storage tag`** (code `not valid`) and **`storage %q does not exist`** (code `not found`): *Trigger:* The storage tag is malformed, or names no storage. *Remediation:* {ref}`List the storages <storage-listing>` and use one of them.
  - **`storage %q is not alive`** (code `not valid`): *Trigger:* The storage is dying. *Remediation:* Attach a storage that is alive.
  - **`storage %q not supported by the charm of unit %q`** (code `not supported`): *Trigger:* The unit's charm declares no storage of that name. *Remediation:* Attach a storage of a name the charm declares.
  - **`storage %q was created for a different charm than unit %q`** (code `not valid`): *Trigger:* The storage serves another charm. *Remediation:* Attach a storage of this charm.
  - **`storage %q kind is not compatible with the charm storage definition of unit %q`** (code `not valid`): *Trigger:* The storage is a volume and the charm asks for a filesystem, or the other way around. *Remediation:* Attach a storage of the kind the charm asks for.
  - **`storage %q size does not meet the charm storage minimum size requirement of unit %q`** (code `not valid`): *Trigger:* The storage is smaller than the charm's minimum. *Remediation:* Attach a larger storage.
  - **`attaching storage %q would exceed the maximum count of %d for storage definition %q of unit %q`** (code `not valid`): *Trigger:* The unit already has the charm's maximum of that storage. *Remediation:* Detach one first.
  - **`storage %q is already attached to unit %q`** (code `already exists`): *Trigger:* The unit uses the storage already. *Remediation:* Use another storage.
  - **`storage %q already has attachments but the charm storage definition of unit %q does not support shared access`** (code `not valid`): *Trigger:* The storage is attached elsewhere and the charm's storage is not shared. *Remediation:* Detach it first, or attach a shared storage.
  - **`storage %q attachments changed while attaching to unit %q, please retry`**, **`unit %q charm changed during storage attachment, please retry`**, and **`unit %q machine changed during storage attachment, please retry`** (code `not valid`): *Trigger:* Another change raced this one. *Remediation:* Retry.
  - **`storage %q is bound to a different machine than unit %q`** (code `not valid`): *Trigger:* The storage is machine-scoped to another machine. *Remediation:* Attach it to a unit on that machine.

(storage-detaching)=
### Detaching storages

```{ibnote}
See also: {ref}`Juju | Detach storage <detach-storage>`
```

Detaching a storage takes it off one unit, or off every unit, without destroying it. What backs it stays, so the storage can attach to another unit later. The client can force the detach and give a time to wait.

The caller, the rules, and the errors are:

- *Caller:* A client, for a user with {ref}`write access <user-access-model-write>` to the model. No block applies.
- *Rules:*
  - A request that names a unit detaches that unit's attachment. A request without a unit detaches every attachment of the storage.
  - A detach that would leave a unit with fewer storages of a name than the charm's minimum is refused.
  - The attachment is marked dying; the unit agent runs {ref}`storage-detaching <hook-storage-detaching>` and gives the storage back.
- *Errors:* The client receives each in the result for the request:
  - **`not authorized for request`** (code `unauthorized access`): *Trigger:* The caller lacks write access to the model. This fails the whole call. *Remediation:* Ask a model administrator.
  - **`max wait time cannot be a negative number`** (code `not valid`): *Trigger:* The request gives a negative wait. This fails the whole call. *Remediation:* Give a positive wait, or none.
  - **`invalid storage tag %q`** and **`invalid unit tag %q`** (code `not valid`): *Trigger:* A tag is malformed. *Remediation:* Use the tags that {ref}`listing <storage-listing>` returns.
  - **`storage %q does not exist`** (code `not found`): *Trigger:* The storage names no storage. *Remediation:* {ref}`List the storages <storage-listing>` and use one of them.
  - **`unit %q does not exist`** (code `not found`): *Trigger:* The unit names no unit. *Remediation:* Name a unit of the model.
  - **`storage %q is not attached to unit %q`** (code `not found`): *Trigger:* The unit does not use the storage. *Remediation:* Detach it without the unit, or from the unit that has it.
  - **`removing storage from unit would violate charm storage %q requirements of having minimum %d storage instances`** (code `not valid`): *Trigger:* The detach would go under the charm's minimum. *Remediation:* Add another storage of that name first, or remove the unit.

(storage-removing)=
### Removing storages

```{ibnote}
See also: {ref}`Juju | Remove storage <remove-storage>`
```

Removing a storage sets it dying so that the controller removes it from the model. A request can destroy what backs it on the cloud instead of releasing it, and can destroy the attachments first.

The caller, the rules, and the errors are:

- *Caller:* A client, for a user with {ref}`write access <user-access-model-write>` to the model. No block applies.
- *Rules:*
  - A removal that is not destroyed only releases what backs the storage: the cloud keeps it, and Juju lets go of it.
  - A destroyed removal also destroys what backs the storages, including a persistent volume, once the attachments are gone.
  - A request that destroys the attachments detaches each of them first, with the same force and wait.
  - The removal is scheduled and finishes after the call returns; a forced removal does not wait for the unit agent.
- *Errors:* The client receives each in the result for the request:
  - **`not authorized for request`** (code `unauthorized access`): *Trigger:* The caller lacks write access to the model. This fails the whole call. *Remediation:* Ask a model administrator.
  - **`max wait time cannot be a negative number`** (code `not valid`): *Trigger:* The request gives a negative wait. *Remediation:* Give a positive wait, or none.
  - **`invalid storage tag`** (code `not valid`): *Trigger:* The tag is malformed. *Remediation:* Use the tag that {ref}`listing <storage-listing>` returns.
  - **`storage %q does not exist`** (code `not found`): *Trigger:* The storage names no storage. *Remediation:* {ref}`List the storages <storage-listing>` and use one of them.

(storage-importing)=
### Importing filesystems

```{ibnote}
See also: {ref}`Juju | Reuse storage <reuse-storage>`
```

Importing a filesystem adopts a filesystem that the provider already knows into the model, as a new storage of one of the unit's charm storage names. The block kind cannot be adopted.

The caller, the rules, and the errors are:

- *Caller:* A client, for a user with {ref}`write access <user-access-model-write>` to the model. No block applies.
- *Rules:*
  - The request names the provider ID of the filesystem, the pool to record it under, and the storage name it will serve. It can force the adoption when the filesystem is in use.
  - The answer names the new storage by its tag.
- *Errors:* The client receives each in the result for the request:
  - **`not authorized for request`** (code `unauthorized access`): *Trigger:* The caller lacks write access to the model. This fails the whole call. *Remediation:* Ask a model administrator.
  - **`storage pool name is not valid`** (code `not valid`) and **`storage pool not found`** (code `not found`): *Trigger:* The pool breaks the name rule, or names no pool. *Remediation:* {ref}`List the pools <storage-pool-listing>` and use one of them.
  - **`storage entity not found in pool`** (code `not found`): *Trigger:* The provider knows no such filesystem under that pool. *Remediation:* Name the filesystem as the provider sees it.
  - **`adopting block storage is not supported`** (code `not supported`): *Trigger:* The request names the block kind. *Remediation:* Import a filesystem, or {ref}`attach <storage-attaching>` a volume instead.
  - **`invalid storage kind`** (code `not valid`): *Trigger:* The request names neither kind. *Remediation:* Name the filesystem kind.

### Provisioning volumes and filesystems

The machine agent provisions the volumes and filesystems of its machines: it watches what is due, asks the provider to create or attach each one, and records what it got. The controller runs one provisioner for the whole model; a machine agent runs one for its own machines.

The caller, the rules, and the errors are:

- *Caller:* A machine agent, for its own machines and the containers on them. A controller's machine agent acts for the whole model.
- *Rules:*
  - `VolumeParams` and `FilesystemParams` return what the provider needs to create or attach each one: the pool's provider, the size, the attributes of the pool, and the tags of the model.
  - `SetVolumeInfo` and `SetFilesystemInfo` record what the provider created: its provider ID, its size, and for a volume its hardware ID, worldwide name, and whether it is persistent. The request cannot change the pool.
  - `SetStatus` sets the status of a volume or a filesystem, with a message and data; the statuses are the eight listed above. The provisioner reports a failure as `error` with the reason.
  - `Life` returns the life of a volume or a filesystem, and `VolumeBlockDevices` returns the block devices of a machine.
  - `RemoveVolumeParams` and `RemoveFilesystemParams` return whether to destroy or release each dying one.
- *Errors:* The agent receives each in the result for the entity:
  - **`permission denied`** (code `unauthorized access`): *Trigger:* The agent acts for a machine that is not its own, or for an entity that does not exist. *Remediation:* Use the machines of the agent.
  - **`volume %q not found`** and **`filesystem %q not found`** (code `not found`): *Trigger:* The tag names no volume or filesystem. *Remediation:* Use the tags that the watcher reported.
  - **`volume %q is not provisioned`** (code `not provisioned`): *Trigger:* The volume has no size recorded yet. *Remediation:* Record the volume's info first.
  - **`pool field must not be set`**: *Trigger:* The request tried to change the pool. *Remediation:* Leave the pool out.

### Attaching volumes and filesystems to machines

The machine agent attaches the volumes and filesystems of its machines. For a volume that the machine cannot see yet, it first plans the attachment, and a cloud that needs it applies the plan.

The caller, the rules, and the errors are:

- *Caller:* A machine agent, for its own machines and the containers on them. A controller's machine agent acts for the whole model.
- *Rules:*
  - `VolumeAttachmentParams` and `FilesystemAttachmentParams` return what the provider needs to attach each one to a machine, including the machine's instance ID and the provider ID of the volume.
  - `SetVolumeAttachmentInfo` records whether the attachment is read-only. It also records the device name, the device link, or the bus address under which the machine sees the volume. The same call records the plan's outcome when one exists.
  - `SetFilesystemAttachmentInfo` records the mount point of a filesystem attachment.
  - `CreateVolumeAttachmentPlans` plans a volume attachment before the volume is on the machine. The plan names the device type — `local` or `iscsi` — plus free-form attributes for the provider, such as an iSCSI target.
  - `SetVolumeAttachmentPlanBlockInfo` records the block device that the plan found on the machine, and `RemoveVolumeAttachmentPlan` drops a plan.
- *Errors:* The agent receives each in the result for the entity:
  - **`permission denied`** (code `unauthorized access`): *Trigger:* The agent acts for a machine that is not its own. *Remediation:* Use the machines of the agent.
  - **`volume attachment for machine %q and volume %q not found`** (code `not found`): *Trigger:* No attachment exists for that volume on that machine. *Remediation:* Attach the volume first.
  - **`plan device type %q not valid`** (code `not valid`): *Trigger:* The plan names a device type other than `local` and `iscsi`. *Remediation:* Name one of the two.
  - **`machine %q not found`** (code `not found`): *Trigger:* The machine names no machine. *Remediation:* Use a machine of the model.

### Reading a unit's storage attachments

The unit agent reads the storages of its unit, and takes them off the unit's record when the charm is done with them. The charm reads the same through the {ref}`storage-get <hook-command-storage-get>`, {ref}`storage-list <hook-command-storage-list>`, and `storage-id` hook commands.

The caller, the rules, and the errors are:

- *Caller:* A unit agent, for its own unit.
- *Rules:*
  - `UnitStorageAttachments` returns the tags of the storages attached to the unit.
  - `StorageAttachments` returns the details of one attachment: its location (the mount point of the filesystem, or the device of the volume), its life, and the machine it is on.
  - `StorageAttachmentLife` returns the life of one attachment, and `RemoveStorageAttachments` marks a dead attachment gone.
  - The {ref}`storage-attached <hook-storage-attached>` hook of a storage runs before the {ref}`install <hook-install>` hook on a machine model, and after the {ref}`start <hook-start>` hook on Kubernetes. The {ref}`storage-detaching <hook-storage-detaching>` hook runs while the storage is being taken off.
- *Errors:* The agent receives each in the result for the entity:
  - **`permission denied`** (code `unauthorized access`): *Trigger:* The agent acts for a unit that is not its own. *Remediation:* Use the unit that the agent runs.
  - **`unit %q not found`** (code `not found`): *Trigger:* The unit names no unit. *Remediation:* Use the unit that the agent runs.
  - **`storage instance %q not found`** and **`storage attachment %q for unit %q not found`** (code `not found`): *Trigger:* The storage names no storage, or the unit does not use it. *Remediation:* Use the attachments that `UnitStorageAttachments` returned.

## Storage and model migration

A {ref}`migration <model-migrating>` moves the storages of a model with their pools, volumes, filesystems, and attachments. The pools move first: the pools that a user made keep their names, a pool whose name breaks the new rule keeps it under the legacy rule, and where a user pool and a provider default pool share a name, the user pool wins. The storages then move with what backs them; on a Kubernetes model the filesystem import must find the persistent volume claim that backs each filesystem by name. See {ref}`Migrating models <model-migrating>`.