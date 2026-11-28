---
myst:
  html_meta:
    description: "Identify Juju storage instances and their backing, inspect attachments, and distinguish detachment, release and destruction."
---

(storage)=
# Storage

```{ibnote}
See also: {ref}`Juju CLI | Manage storage <manage-storage>`
```

```{ggarch}
:file: ../juju.ggarch
:view: Storage instance and backing
:alt: In this example a unit uses an attachment to connect to the highlighted storage instance. That instance provides a filesystem built on a volume.
:caption: A storage instance can remain in the model after a unit stops using it. Here the unit uses an attachment to access a filesystem backed by a volume. Detaching the unit ends its access to that filesystem.
```

In Juju, **storage** is a [block device](https://docs.openstack.org/doc-contrib-guide/common/glossary.html#term-block-device) or [filesystem](https://en.wikipedia.org/wiki/File_system) that a {ref}`charm <charm>` requests for a {ref}`unit <unit>` to use. Juju tracks each allocation as a **storage instance** in the {ref}`model <model>`. A unit uses the instance through an {ref}`attachment <storage-ownership-and-attachments>`. When the attachment ends, the instance can remain in the model.

For block storage, the unit gets direct access to a volume: storage space the machine can read and write like a disk. An [Amazon EBS volume](https://docs.aws.amazon.com/ebs/latest/userguide/ebs-volumes.html) is one example. For filesystem storage, Juju can build a filesystem on a volume. Whether the data can outlive the {ref}`machine <machine>` depends on {ref}`what backs the storage instance <storage-backing>`.

(storage-names-and-instance-ids)=
## Storage names and instance IDs

```{ibnote}
See also: {ref}`Juju CLI | Find a charm's storage names <find-a-charms-storage-names>`
```

A **storage name** is a key in the `storage` mapping of the deployed charm revision's `metadata.yaml`. For example, a charm can declare `data` as a filesystem. Each charm defines its own names and can declare optional storage before any instances exist.

Juju assigns each instance an ID consisting of its storage name and a model-allocated number, such as `data/7`. The number is not the unit number. The ID identifies the Juju instance, not the provider's volume, filesystem or Kubernetes PersistentVolume (PV).

(storage-backing)=
## Storage backing

The charm declares what kind of storage the unit needs. For block storage, Juju provisions a volume and gives the unit a device path. For filesystem storage, a {ref}`storage provider <storage-providers>` can supply a filesystem directly, or Juju can make one on a volume. In the latter case the filesystem, volume and instance have distinct IDs. The unit uses the filesystem at its mount point.

A machine-scoped backing belongs to a particular machine. A model-scoped backing can outlive that machine. If there is a volume, its scope determines the storage instance's scope for removal; otherwise the filesystem's scope does. For example, a filesystem made on a machine from a model-scoped volume has model scope.

(storage-pool)=
## Storage pool

```{ibnote}
See also: {ref}`manage-storage-pools`
```

A **storage pool** is a mechanism for administrators to define sources of storage that they will use to satisfy application storage requirements.

A single pool might be used for storage from units of many different applications - it is a resource from which different stores may be drawn.

A pool describes {ref}`storage provider <storage-provider>`-specific parameters for creating storage, such as performance (e.g. IOPS), media type (e.g. magnetic vs. SSD), or durability.

For many providers, there will be a shared resource where storage can be requested (e.g. for Amazon EC2, `ebs`). Creating pools there maps provider specific settings into named resources that can be used during deployment.

Pools defined at the model level are easily reused across applications. Pool creation requires a pool name, the provider type and attributes for configuration as space-separated pairs, e.g. tags, size, path, etc.

(storage-provider)=
(storage-providers)=
## Storage providers

A **storage provider** supplies backing for storage instances. Machine clouds have three common providers. On Kubernetes, the `kubernetes` provider uses a PersistentVolumeClaim (PVC) for a unit's attachment to a PV. The `rootfs` and `tmpfs` providers use pod `emptyDir` volumes.

(list-of-storage-providers)=
### List of storage providers

(cloud-specific-storage-providers)=
(storage-provider-cloud-specific)=
#### Cloud-specific storage providers

The {ref}`reference for each supported cloud <list-of-supported-clouds>` identifies its storage providers. For example, the {ref}`EBS provider <storage-provider-ebs>` supplies volumes on AWS.

(loop)=
(loop-storage-provider)=
#### `loop` storage provider

On machines, `loop` supplies a block device backed by a file on that machine using a [loop device](https://en.wikipedia.org/wiki/Loop_device).

(rootfs)=
(rootfs-storage-provider)=
#### `rootfs` storage provider

On machines, `rootfs` supplies a directory on the machine's [root filesystem](https://www.kernel.org/doc/Documentation/filesystems/ramfs-rootfs-initramfs.txt). The directory remains after Juju removes its filesystem record. Data in the directory can still be lost with the machine.

(tmpfs)=
(tmpfs-storage-provider)=
#### `tmpfs` storage provider

On machines, `tmpfs` supplies a [temporary filesystem](https://en.wikipedia.org/wiki/Tmpfs). Its contents are lost on unmount.

(storage-ownership-and-attachments)=
## Storage ownership and attachments

A **storage attachment** connects one instance to one unit. It has its own lifecycle. An instance provisioned for a unit can also have that unit as its owner. Attaching an existing instance to a machine unit does not make the unit its owner. When Juju deletes the owning unit's attachment, it also clears that ownership.

The distinction matters when {ref}`removing a unit <removing-a-unit-with-storage>` and when reading model status: a status query filtered to particular units selects storage by **owner**, not by every attachment. An ownerless instance that disappears from filtered status can still be in the {ref}`model inventory <listing-storage-instances>`.

(storage-directive)=
(storage-requests)=
## Storage requests

```{ibnote}
See also: {ref}`Juju CLI | Request storage for an application <request-storage-for-an-application>`
```

A **storage directive** specifies how many instances of one {ref}`storage name <storage-names-and-instance-ids>` each unit needs, their size and their source. By default, Juju requests the charm's minimum count, which can be zero. If the charm omits the count range, it requires one instance. The default size is the charm's minimum size, or 1 GiB if it declares none. The model supplies the default source for each kind of storage.

An override must use a name declared by the charm, respect its count range and minimum size, and select a source that supports its kind. Juju refuses required shared storage and skips optional shared declarations when creating default directives.

Each unit has directives for its current charm revision. API clients can {ref}`read an application's storage requests <reading-an-applications-storage-requests>`.

(storage-mount-points)=
## Storage mount points

On machines, Juju preserves a filesystem attachment's existing mount point. For a new attachment, Juju uses the charm's exact location if it specifies one and allows at most one instance of this storage name per unit. Otherwise Juju appends the attachment's UUID to the specified directory, or to `/var/lib/juju/storage` if the charm specifies none. {ref}`Inspect the attachment <inspecting-a-storage-instance>` to find its mount point.

(adding-storage-to-a-unit)=
## Adding storage to a unit

```{ibnote}
See also: {ref}`Juju CLI | Add storage to a unit <add-storage-to-a-unit>`
```

Through the Storage API, a client can request additional instances of a storage name on an existing machine or Kubernetes unit. Juju uses the unit's requested size and source unless the caller overrides them. It counts existing attachments and requested instances against the charm's maximum. Success returns the new IDs. Provisioning and the {ref}`storage-attached hook <hook-storage-attached>` follow separately. A request for several names can succeed for some and fail for others.

**Who can do this:** A user with at least {ref}`model write access <user-access-model-write>` while model changes are enabled.

**Errors:** Adding storage can return these errors:

- *Error message:* `storage name "<name>" not supported by charm`. *Trigger:* The unit's current revision has no directive for that name. *What to do:* Check its {ref}`storage names <storage-names-and-instance-ids>`.
- *Error message:* `storage directive "<name>" request count <count> exceeds the charm's maximum count of <maximum>`. *Trigger:* The additional instances would exceed the charm's limit. *What to do:* Inspect existing attachments and the IDs already returned before choosing a smaller request.

(importing-a-filesystem-as-storage)=
## Importing a filesystem as storage

```{ibnote}
See also: {ref}`Juju CLI | Import a filesystem as storage <import-a-filesystem-as-storage>`
```

Importing adopts a provider's existing filesystem into the model under a charm storage name. Juju assigns it a new, detached instance ID. The caller identifies the pool and supplies the provider ID of the filesystem or of the volume on which the filesystem was built. Either form produces a filesystem instance.

Import requires a provider that supports adoption and a model-scoped filesystem or backing volume. The provider acts before Juju records the instance. If that write fails, the provider object may already have changed.

**Who can do this:** A user with at least {ref}`model write access <user-access-model-write>`.

**Errors:** Importing a filesystem can return these errors:

- *Error message:* `storage pool not found`. *Trigger:* The named pool is missing from the selected model. *What to do:* Check the model and pool name.
- *Error message:* `storage entity not found in pool`. *Trigger:* The provider cannot find the supplied object. *What to do:* Check the provider ID against its inventory.
- *Error message:* Ends with `storage filesystem with provider id "<id>" is already adopted`. *Trigger:* The filesystem already has a Juju record. *What to do:* Find the instance in the {ref}`filesystem inventory <listing-filesystems-backing-storage>` before attempting another import.

(listing-storage-instances)=
## Listing storage instances

```{ibnote}
See also: {ref}`Juju CLI | List storage instances <list-storage-instances>`
```

The inventory lists storage instances across all applications in the model, including detached instances. The Storage API requires at least one filter record; each returns the same inventory. Use {ref}`exact inspection <inspecting-a-storage-instance>` for a known ID.

**Who can do this:** A user with at least {ref}`model read access <user-access-model-read>` or a {ref}`controller superuser <user-access-controller-superuser>`.

**Errors:** Listing storage instances can return these errors:

- *Error message:* `not authorized for request`. *Trigger:* The caller lacks read access to this inventory. *What to do:* Check the selected model and the caller's grant.
- *Error message:* `at least one filter is required`. *Trigger:* The API request contains no filter record. *What to do:* Supply one empty filter to list the model's instances.

(inspecting-a-storage-instance)=
## Inspecting a storage instance

```{ibnote}
See also: {ref}`Juju CLI | Inspect a storage instance <inspect-a-storage-instance>`
```

An exact lookup returns one result for each requested ID, in order. **Life** describes the instance itself. **Status** describes the filesystem when available, otherwise its backing volume. A missing status timestamp can appear as the Unix epoch. Each attachment has its own life and, when available, its unit, machine and location.

The `persistent` field comes from the backing volume. It is false when there is no backing volume, even if the provider could retain the filesystem. Before retiring a unit, machine or model, {ref}`identify the backing <identify-storage-backing>` and check whether it survives removal. The flag alone cannot answer that question.

**Who can do this:** A user with at least {ref}`model read access <user-access-model-read>` or a {ref}`controller superuser <user-access-controller-superuser>`.

**Errors:** Inspecting storage can return these errors:

- *Error message:* `invalid storage entity tag <index>`. *Trigger:* The API cannot parse the tag at that position in the request. *What to do:* Construct the storage tag from an ID in the {ref}`model inventory <listing-storage-instances>`.
- *Error message:* `tag kind "<kind>" not supported`. *Trigger:* The tag identifies another kind of entity. *What to do:* Supply a storage tag for the instance.
- *Error message:* `storage instance "<id>" not found`. *Trigger:* The ID is valid but absent from the selected model. *What to do:* Check the ID and model before retrying.

(listing-filesystems-backing-storage)=
## Listing filesystems backing storage

```{ibnote}
See also: {ref}`Juju CLI | Identify storage backing <identify-storage-backing>`
```

A filesystem inventory can include its provider ID, mount point, backing volume ID and attachments. A filesystem need not have a volume. By default the API returns all filesystems; selecting machines returns filesystems attached to those machines, not every filesystem they own.

**Who can do this:** A user with at least {ref}`model read access <user-access-model-read>` or a {ref}`controller superuser <user-access-controller-superuser>`.

**Errors:** Listing filesystems for a machine can return this error:

- *Error message:* `machine "<id>" not found`. *Trigger:* The machine is absent from this model. *What to do:* Check the machine ID. A known machine with no matching attachments returns an empty result.

(listing-volumes-backing-storage)=
## Listing volumes backing storage

```{ibnote}
See also: {ref}`Juju CLI | Identify storage backing <identify-storage-backing>`
```

With no machine selected, the volume inventory covers the whole model, including unattached volumes. To find an unattached volume, check its **attachments**, not whether it has a Juju storage instance ID: a detached instance can still have a volume. A machine selection returns volumes attached to that machine. The provider ID identifies the disk in the cloud.

**Who can do this:** A user with at least {ref}`model read access <user-access-model-read>` or a {ref}`controller superuser <user-access-controller-superuser>`.

**Errors:** Listing volumes for a machine can return this error:

- *Error message:* `machine "<id>" not found`. *Trigger:* The machine is absent from this model. *What to do:* Check its ID. If the request selected several machines, inspect each result separately.

(attaching-storage-to-a-unit)=
## Attaching storage to a unit

```{ibnote}
See also: {ref}`Juju CLI | Attach existing storage to a unit <attach-existing-storage-to-a-unit>`
```

Attaching storage connects an existing instance to a unit. Both must still be in the `alive` stage of their lifecycle: removal must not have started for either. The storage name and kind must match the unit's charm definition. Juju checks the minimum size using the known filesystem or volume size when nonzero, otherwise the requested size. It also checks the charm's count and sharing limits and the instance's machine binding. If the instance has no recorded charm name, it can acquire one on first attachment. Success records the attachment; the unit's mount or device path becomes available separately.

**Who can do this:** A user with at least {ref}`model write access <user-access-model-write>` while model changes are enabled.

**Errors:** Attaching storage can return these errors:

- *Error message:* `storage "<id>" was created for a different charm than unit "<unit>"`. *Trigger:* The recorded charm name differs from the unit's charm. *What to do:* Choose a unit running the matching charm.
- *Error message:* `attaching storage "<id>" would exceed the maximum count of <maximum> for storage definition "<name>" of unit "<unit>"`. *Trigger:* The unit has reached the charm's limit. Juju checks this before checking for a duplicate attachment. *What to do:* Inspect the unit's current attachments.
- *Error message:* `storage "<id>" is bound to a different machine than unit "<unit>"`. *Trigger:* The instance is tied to another machine. *What to do:* Choose a compatible unit on that machine.
- *Error message:* Ends with `please retry`. *Trigger:* The attachments, unit charm or unit placement changed during the request. *What to do:* Check the instance and the unit's current placement and revision before retrying.

(detaching-storage-from-a-unit)=
## Detaching storage from a unit

```{ibnote}
See also: {ref}`Juju CLI | Detach storage <detach-storage>`
```

```{ggarch}
:file: ../juju.ggarch
:view: Detaching a storage instance
:alt: The controller marks an attachment dying. The unit agent handles the detaching hook and commits it before attachment cleanup.
:caption: After normal detachment of a previously attached instance, the unit loses access and the instance remains in the model.
```

Detachment schedules removal of the attachment. A client can target an instance and unit together, or request every attachment of an instance. With no unit specified, an already detached instance needs no work. On the ordinary path, committing the {ref}`storage-detaching hook <hook-storage-detaching>` initiates attachment cleanup. An attachment whose attached hook was never committed can disappear without a paired hook.

**Who can do this:** A user with at least {ref}`model write access <user-access-model-write>`.

**Errors:** Detaching storage can return these errors:

- *Error message:* Ends with `removing storage from unit would violate charm storage "<name>" requirements of having minimum <count> storage instances`. *Trigger:* Detachment would take the unit below the charm's minimum, including when forced. *What to do:* Keep the required attachment or satisfy the minimum with another instance within the charm's count range.
- *Error message:* `storage "<id>" is not attached to unit "<unit>"`. *Trigger:* The caller selected an instance and unit with no shared attachment. *What to do:* Inspect the instance's current attachments. If an earlier request targeted all units, some detachments may already be in progress.

(removing-or-releasing-a-storage-instance)=
## Removing or releasing a storage instance

```{ibnote}
See also: {ref}`Juju CLI | Remove or release a storage instance <remove-or-release-a-storage-instance>`
```

Both removal and release retire the Juju instance ID. Removal requests destruction of the backing; release leaves it outside Juju's management. A provider failure can delay cleanup. Forced cleanup can discard Juju's records before the provider finishes. To establish what remains, the operator needs the provider's inventory.

**Who can do this:** A user with at least {ref}`model write access <user-access-model-write>`.

**Errors:** Removing or releasing storage can return this error:

- *Error message:* Ends with `storage instance still attached`. *Trigger:* At least one attachment is still alive or dying. A request to detach first can start some detachments before removal fails. *What to do:* Inspect the attachments and wait for detachment to finish before retrying.

