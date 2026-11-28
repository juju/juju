---
myst:
  html_meta:
    description: "Learn to add, attach, detach, and remove storage in Juju. Manage persistent storage for applications across cloud providers."
---

(manage-storage)=
# How to manage storage

```{ibnote}
See also: {ref}`storage`
```

Use the {ref}`Juju CLI <juju-cli>` to request storage for a unit, find its filesystem or volume, and decide what to keep when the unit no longer needs it. Run the commands in the {ref}`model you want to work in <switch-to-a-different-model>`.

(find-a-charms-storage-names)=
## Find a charm's storage names

```{ibnote}
See also: {ref}`Storage names and instance IDs <storage-names-and-instance-ids>`
```

To find the storage names a unit accepts, check the `storage:` mapping in its deployed {ref}`charm revision <charm-revision>`. First identify that revision:

```text
juju status --format=yaml
```

Under `applications`, use `charm` and `charm-rev`, not the application alias. During a refresh, use the unit's `charm` if it differs from the application's.

```{ibnote}
See more: {ref}`command-juju-status`
```

For a published charm, download that revision and open `metadata.yaml`. For example, for PostgreSQL revision 1217 on amd64:

```text
juju download postgresql --revision 1217 --filepath postgresql-1217.charm
unzip -p postgresql-1217.charm metadata.yaml
```

Look for the keys under `storage:`. This revision declares `pgdata` and requires exactly one filesystem per unit. For a local charm, open `metadata.yaml` in the matching `.charm` archive instead. Use the declaration even if `juju storage` lists no instances yet.

```{ibnote}
See more: {ref}`command-juju-download`
```

(request-storage-for-an-application)=
## Request storage for an application

```{ibnote}
See also: {ref}`Storage requests <storage-requests>`
```

To request storage when deploying an application, pass `--storage` to `juju deploy` with the {ref}`charm's storage name <find-a-charms-storage-names>` and the size you need. For example, to request one 10 GiB filesystem per unit for PostgreSQL revision 1217 on Ubuntu 22.04, amd64:

```text
juju deploy postgresql --channel 14/stable --revision 1217 --storage pgdata=10G
```

For more than one name, repeat `--storage`. Within a directive, keep the source, count and size in one comma-separated argument without spaces. A bare integer specifies a count; a size has a unit suffix. {ref}`List the instances <list-storage-instances>` to find the resulting Juju IDs.

```{ibnote}
See more: {ref}`command-juju-deploy`
```

(add-storage)=
(add-storage-to-a-unit)=
## Add storage to a unit

```{ibnote}
See also: {ref}`Adding storage to a unit <adding-storage-to-a-unit>`
```

To add storage to an existing machine unit, run `juju add-storage` with the unit and a {ref}`storage name its charm accepts <find-a-charms-storage-names>`. Check that the charm allows another instance and the device can attach to the unit's machine. For EBS, check the {ref}`availability zone <storage-provider-ebs>` first. For example, to request another 32 GiB EBS device for `ceph-osd/0`:

```text
juju add-storage ceph-osd/0 osd-devices=ebs,32G,1
```

To use the unit's existing size and source, pass just `osd-devices`. Note the new ID in the output and {ref}`inspect the attachment <inspect-a-storage-instance>` before using the device.

```{ibnote}
See more: {ref}`command-juju-add-storage`
```

(reuse-storage)=
(import-a-filesystem-as-storage)=
## Import a filesystem as storage

```{ibnote}
See also: {ref}`Importing a filesystem as storage <importing-a-filesystem-as-storage>`
```

To bring an existing filesystem into the selected model, run `juju import-filesystem` with its pool, provider ID and the storage name expected by the destination charm. For a detached instance already listed in this model, {ref}`attach the existing instance <attach-existing-storage-to-a-unit>`.

```{warning}
Check that no other model or workload is using the filesystem. If an import fails, inspect the provider object before retrying; the provider may already have changed it.
```

`````{tabs}

````{tab} Kubernetes
Check that the PersistentVolume (PV) is unbound and its reclaim policy is `Retain`. If it has a claim reference, inspect the PersistentVolumeClaim and its owner; an idle pod does not make the PV available. For example, to import an eligible PV named `pv-data-001` for a charm that declares `pgdata`:

```text
juju import-filesystem kubernetes pv-data-001 pgdata
```
````

````{tab} Machines
For example, to import an unused EBS volume that contains a compatible filesystem, substitute its actual volume ID for `vol-123456` and the charm's storage name for `pgdata`:

```text
juju import-filesystem ebs vol-123456 pgdata
```
````

`````

The imported instance is detached. Use the new Juju ID in the `imported storage` message to {ref}`inspect the instance <inspect-a-storage-instance>`.

```{ibnote}
See more: {ref}`command-juju-import-filesystem`
```

(view-the-available-storage)=
(list-storage-instances)=
## List storage instances

```{ibnote}
See also: {ref}`Listing storage instances <listing-storage-instances>`
```

To list storage instances in the selected model, run:

```text
juju storage
```

Find the storage ID and the unit, if attached. Use `--format=yaml` for structured output. To look up a particular `name/number`, {ref}`inspect the instance <inspect-a-storage-instance>` instead of passing its ID to `juju storage`.

```{ibnote}
See more: {ref}`command-juju-storage`
```

(view-storage-details)=
(inspect-a-storage-instance)=
## Inspect a storage instance

```{ibnote}
See also: {ref}`Inspecting a storage instance <inspecting-a-storage-instance>`
```

To inspect a storage instance, pass its ID from the {ref}`model inventory <list-storage-instances>` to `juju show-storage`. For example, to check `osd-devices/2`:

```text
juju show-storage osd-devices/2
```

Read `status.current` and look for the intended unit under `attachments.units`. Once available, `location` gives the device path or mount point. To inspect several IDs, separate them with spaces. If an ID is missing, check its spelling and the selected model.

```{ibnote}
See more: {ref}`command-juju-show-storage`
```

(identify-storage-backing)=
## Identify storage backing

```{ibnote}
See also: {ref}`Listing filesystems backing storage <listing-filesystems-backing-storage>`, {ref}`Listing volumes backing storage <listing-volumes-backing-storage>`
```

To find the provider ID behind a Juju storage instance, list its filesystem or volume. The CLI marks both backing-inventory options as deprecated. For example, to find filesystems attached to machine `0` and see whether one is backed by a volume:

```text
juju storage --filesystem --format=yaml 0
```

To find the provider disk ID for a volume attached to machine `0`:

```text
juju storage --volume --format=yaml 0
```

Omit the machine ID to include unattached backing in the model-wide search. Select one kind of backing per command. Record the provider ID before releasing the storage.

```{ibnote}
See more: {ref}`command-juju-storage`
```

(refreshing-an-application-with-storage)=
(refresh-an-application-with-storage)=
## Refresh an application with storage

```{ibnote}
See also: {ref}`Storage requests <storage-requests>`
```

To request storage for a new charm revision, compare the `storage:` mappings in the deployed and target {ref}`charm archives <find-a-charms-storage-names>`. If the target adds a required storage name or changes its requirements, pass a compatible `--storage name=directive` to `juju refresh` for each affected name.

Check the revision on each unit as the refresh proceeds. If `juju refresh` reports that the charm is already up to date, the `--storage` argument alone has not applied a new request. To move the data or increase the capacity of an existing disk, plan that work separately.

```{ibnote}
See more: {ref}`command-juju-refresh`
```

(attach-storage)=
(attach-existing-storage-to-a-unit)=
## Attach existing storage to a unit

```{ibnote}
See also: {ref}`Attaching storage to a unit <attaching-storage-to-a-unit>`
```

To attach detached storage to an existing machine unit, run `juju attach-storage` with the target unit and the Juju storage ID. Check the charm's storage name, kind, size and remaining count, the device's placement, and whether the workload can use its contents.

For example, to attach a compatible `osd-devices/2` to `ceph-osd/1` and check its location:

```text
juju attach-storage ceph-osd/1 osd-devices/2
juju show-storage osd-devices/2
```

```{ibnote}
See more: {ref}`command-juju-attach-storage`, {ref}`command-juju-show-storage`
```

To attach a detached device while adding a machine unit to an existing `ceph-osd` application, request a single unit:

```text
juju add-unit ceph-osd --attach-storage osd-devices/2
```

```{ibnote}
See more: {ref}`command-juju-add-unit`
```

To deploy a new machine application with a retained `pgdata/0` compatible with PostgreSQL revision 1217:

```text
juju deploy postgresql --channel 14/stable --revision 1217 --attach-storage pgdata/0
```

```{ibnote}
See more: {ref}`command-juju-deploy`
```

(removing-a-unit-with-storage)=
(keep-data-when-retiring-a-unit-machine-or-model)=
(choose-which-storage-to-keep)=
## Choose which storage to keep

```{ibnote}
See also: {ref}`Storage ownership and attachments <storage-ownership-and-attachments>`, {ref}`Storage backing <storage-backing>`
```

To choose which storage to keep before removing its unit, machine or model, {ref}`identify its backing <identify-storage-backing>` while it is still visible in Juju. Record the Juju and provider IDs and verify a backup outside the resource you plan to remove.

- **Unit or application**: Before {ref}`removing a machine unit <remove-a-unit>` or {ref}`its application <remove-an-application>`, check which storage the units own. Omit `--destroy-storage` for storage you intend to keep, and check whether its machine will also be removed. On Kubernetes, {ref}`scaling down <remove-a-unit>` takes a count; the operator cannot select which unit's volume to retire.
- **Machine**: Before {ref}`removing a machine <remove-a-machine>`, inspect the model-wide backing inventory, including when the removal preview is empty. Back up machine-local files before removing their machine.
- **Model**: When {ref}`destroying a model <destroy-a-model>`, select `--release-storage` to keep eligible backing outside Juju. Include unattached and unprovisioned storage in your check. Back up machine-local files separately.
- **Controller**: Before {ref}`destroying a controller <destroy-a-controller>` and its models, check storage in every model. Use `--release-storage` with `--destroy-all-models` if you intend to retain eligible backing.

(detach-storage)=
## Detach storage

```{ibnote}
See also: {ref}`Detaching storage from a unit <detaching-storage-from-a-unit>`
```

To detach storage in a machine model, pass the Juju storage ID to `juju detach-storage`. It targets all units using that instance.

```{warning}
Stop the workload from using the device first and check that each unit will still have the storage its charm requires. If you are moving data, complete and verify the workload migration before detaching the original device. Detaching does not copy data.
```

For example, to detach `osd-devices/2` and monitor its attachments in the selected model:

```text
juju detach-storage osd-devices/2
watch juju show-storage osd-devices/2
```

When the old units no longer appear under `attachments.units`, press `Ctrl+C`. To reuse the instance in this model, keep its Juju ID and leave it detached.

```{ibnote}
See more: {ref}`command-juju-detach-storage`, {ref}`command-juju-show-storage`
```

(remove-storage)=
(remove-or-release-a-storage-instance)=
## Remove or release a storage instance

```{ibnote}
See also: {ref}`Removing or releasing a storage instance <removing-or-releasing-a-storage-instance>`
```

To stop managing a storage instance, use `juju remove-storage`. Choose whether you want to keep the provider's volume or filesystem. If you intend to reattach the same Juju ID, leave the instance detached instead.

```{warning}
Before removing the instance, verify your backup or completed workload migration. Check the selected model and Juju ID, record the {ref}`provider ID <identify-storage-backing>`, and wait for {ref}`detachment <detach-storage>` to finish. Without `--no-destroy`, Juju requests destruction of the backing.
```

To release a detached `osd-devices/2` and keep its backing outside Juju:

```text
juju remove-storage --no-destroy osd-devices/2
```

To discard the backing of a detached `osd-devices/2` whose data you no longer need:

```text
juju remove-storage osd-devices/2
```

Check that the instance has left the {ref}`model inventory <list-storage-instances>`. Then check the provider inventory using the recorded ID. On Kubernetes, check the PV, its claim and its reclaim policy. A missing Juju ID is not proof of erasure.

```{ibnote}
See more: {ref}`command-juju-remove-storage`
```

