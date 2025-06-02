(manage-storage)=
# How to manage storage

<!-- This doc has been crafted from https://discourse.charmhub.io/t/how-to-define-and-use-storage/1079 and https://discourse.charmhub.io/t/how-to-remove-storage/5890 -->

> See also: {ref}`storage`

This document shows how to manage storage. This will enable you to allocate resources at a granular level and can be useful in optimizing the deployment of an application. The level of sophistication is limited by your cloud (whether it supports dynamic storage or storage configuration attributes),
 and by the charm in charge of that application (whether it supports storage persistence, additional cache, etc.).

(add-storage)=
## Add storage

Assuming the storage provider supports it, you can create and attach storage instances to units in a specific way by using `juju add-storage`.

First, identify the application unit to which you wish to attach the storage. As an example, suppose we want to target unit 0 of `ceph-osd`, that is, `ceph-osd/0`.

Second, prepare a storage constraint for your desired storage. For example, given the `ceph-osd` charm and assuming we are in an AWS model, we might have `osd-devices=ebs, 32G, 1`.

Finally, run the `add-storage` command passing as arguments the unit to which storage is to be attached and the storage constraint. Drawing on our steps before, we can create a 32GiB EBS volume and attach it to unit 'ceph-osd/0' as its OSD storage as below:

```text
juju add-storage ceph-osd/0 osd-devices=ebs,32G,1
```

Juju will ensure the storage is allowed to attach to the unit's machine.

```{caution}
The above only works if the volume is in the same availability zone as the instance. This is a requirement that comes from the `ebs` storage provider. See {ref}`dynamic-storage`.
```

> See more: {ref}`command-juju-add-storage`, {ref}`storage-constraint`


You can also create and attach storage during deployment by running the `deploy` command with a `--storage` option followed by your desired storage constraint. For example, suppose you have an AWS model, you want to deploy the `postgresql` charm on it, and you've already identified a suitable storage constraint of the form `pgdata=iops,100G` (suppose iops is a version of `ebs` with 30 IOPS). You can use all of these in one go as below. This will create a 3000 IOPS storage volume (100GiB x 30IOPS/GiB (x 1)) and attach it to the newly deployed PostgreSQL as its database storage.

```text
juju deploy postgresql --storage pgdata=iops,100G
```

```{caution}
Charms might specify a maximum number of storage instances. For example, in the case of the `postgresql` charm, a maximum of one is allowed for 'pgdata'. If an attempt is made to exceed it, Juju will return an error.
```

> See more: {ref}`command-juju-deploy`

And you can also create and attach storage while upgrading a charm.

```{note}
Specifying new constraints may be necessary when upgrading to a revision of a charmed operator that introduces new, required, storage options.
```


The logic is entirely parallel to the case where this was done while deploying a charm---you do this by running the `refresh` command with the `--storage` option followed by a suitable  storage constraint, e.g., `pgdata=10G`, as shown below. This will change any existing constraints or define new ones (for example, in the case where the storage option did not exist in the version of the charm before the upgrade). If you don't specify any constraints, the defaults will kick in.

```text
juju refresh postgresql --storage pgdata=10G
```

> See more: {ref}`command-juju-refresh`

(view-the-available-storage)=
## View the available storage

TBA

> See more: {ref}`command-juju-storage`


(view-storage-details)=
## View storage details

TBA

> See more: {ref}`command-juju-show-storage`

(detach-storage)=
## Detach storage

If the storage is dynamic, you can detach it from units by running `juju detach-storage` followed by the unit you want to detach. For example, to detach OSD device 'osd-devices/2' from a Ceph unit, do:

```text
juju detach-storage osd-devices/2
```

```{caution}
Charms might define a minimum number of storage instances. For example, the `postgresql` charm specifies a minimum of zero for its 'pgdata'. If detaching storage from a unit would bring the total number of storage instances below the minimum, Juju will return an error.
```

```{note}
Detaching storage from a unit does not destroy the storage.
```

> See more: {ref}`command-juju-detach-storage`, {ref}`dynamic-storage`

(attach-storage)=
## Attach storage

Detaching storage does not destroy the storage. In addition, when a unit is removed from a model, and the unit has dynamic storage attached, the storage will be detached and left intact. This allows detached storage to be re-attached to an existing unit. This can be done during deployment / when you're adding a unit / at any time, as shown below:

To deploy PostgreSQL with (detached) existing storage 'pgdata/0':

```text
juju deploy postgresql --attach-storage pgdata/0
```

> See more: {ref}`command-juju-deploy`

To add a new Ceph OSD unit with (detached) existing storage 'osd-devices/2':

```text
juju add-unit ceph-osd --attach-storage osd-devices/2
```
```{note}

The `--attach-storage` and `-n` flags cannot be used together.

```

> See more: {ref}`command-juju-add-unit`


To attach existing storage 'osd-devices/7' to existing unit 'ceph-osd/1':

```text
juju attach-storage ceph-osd/1 osd-devices/7
```

> See more: {ref}`command-juju-attach-storage`

(reuse-storage)=
## Reuse storage

If you've destroyed a model but kept the storage, you'll likely want to reuse it. You can do this by running the `juju import-filesystem` command followed by the storage provider, the provider id, and the storage name. For example, given a LXD model (with the storage provider `lxd`), a provider id of `juju:juju-7a544c-filesystem-0`, and a storage name of `pgdata`, this is as below:

```text
juju add-model default
juju import-filesystem lxd juju:juju-7a544c-filesystem-0 pgdata
```

```{important}
The determination of the provider ID  is dependent upon the cloud type. Above, it is given by the backing LXD pool and the volume name (obtained with `lxc storage volume list <lxd-pool`), all separated by a `:`. A provider ID from another cloud may look entirely different.
```

<!--
It is not possible to add new storage to a model without also attaching it to a unit. However, with the `juju import-filesystem` command, you can add storage to a model that has been previously released from a removed model.
-->

> See more: {ref}`command-juju-import-filesystem`, {ref}`storage-provider-lxd`

(remove-storage)=
## Remove storage
> See also: {ref}`removing-things`


The underlying cloud's storage resource is normally destroyed by first detaching it and then using `juju remove-storage`. For example, assuming the 'osd-devices/3' storage instance has already been detached, the code below will remove it from the model. It will also be automatically destroyed on the cloud provider.

```text
juju remove-storage osd-devices/3
```

````{dropdown} Expand to see a scenario where you use this to upgrade your storage

To upgrade the OSD journal of Ceph unit 'ceph-osd/0' from magnetic to solid state (SSD) and dispose of the unneeded original journal 'osd-journals/0':

```text
juju add-storage ceph-osd/0 osd-journals=ebs-ssd,8G,1
juju detach-storage osd-journals/0
juju remove-storage osd-journals/0
```

````

```{important}
You can also remove storage from the model and prevent it being destroyed on the cloud provider by passing the `--no-destroy` flag. However, be wary of using  this option as Juju will lose sight of the volume and it will only be visible from the cloud provider.
```

If an attempt is made to either attach or remove storage that is currently in use (i.e. it is attached to a unit) Juju will return an error. To remove currently attached storage from the model the `--force` option must be used. For example,

```text
juju remove-storage --force pgdata/1
```

> See more: {ref}`command-juju-remove-storage`

Finally, a model cannot be destroyed while storage volumes remain without passing a special option (`--release-storage` to detach all volumes and `--destroy-storage` to remove all volumes). You can handle this by either destroying the storage or releasing it (so you can later attach it to something else), as shown below:

```text
# Destroy the model along with all existing storage volumes:
juju destroy-model default --destroy-storage

# Destroy the model while keeping intact all the storage volumes:
juju destroy-model default --release-storage
```

> See more: {ref}`command-juju-destroy-model`

Naturally, this applies to the removal of a controller as well.

To destroy a controller (and its models) along with all existing storage volumes:

```text
# Destroy the controller along with all existing storage volumes:
juju destroy-controller lxd-controller --destroy-all-models --destroy-storage

# Destroy the controller while keeping intact all the storage volumes:
juju destroy-controller lxd-controller --destroy-all-models --release-storage
```

> See more: {ref}`command-juju-destroy-controller`

