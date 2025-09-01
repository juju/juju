(storage)=
# Storage

```{ibnote}
See also: {ref}`manage-storage`
```

In Juju, **storage** refers to a data volume that is provided by a {ref}`cloud <cloud>`.

Depending on how things are set up during deployment, the data volume can be machine-dependent (e.g., a directory on disk tied to the machine which goes away if the unit is destroyed) or machine-independent (i.e., it can outlive a machine and be reattached to another machine).

Most storage can be dynamically added to, and removed from, a unit. However, by their nature, some types of storage cannot be dynamically managed (see {ref}`storage-provider-maas`). Also, certain cloud providers may impose restrictions when attaching storage (see {ref}`storage-provider-ebs`).

(storage-directive)=
## Storage directive

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

The order of the arguments does not actually matter -- they are identified based on a regex (pool names must start with a letter and sizes must end with a unit suffix).

If at least one storage directive component is specified, the following default values come into effect:

* `<pool>`: the default storage pool.
* `<count>`: the minimum number required by the charm, or '1' if the storage is optional
* `<size>`: determined from the charm's minimum storage size, or 1GiB if the charm does not specify a minimum

In the absence of any explicit storage directive, the storage will be put on the root filesystem (`rootfs`). <!--I'm guessing this takes care of `<pool>`. What about the other values?-->

(storage-pool)=
## Storage pool

```{ibnote}
See also: {ref}`manage-storage-pools`
```

<!-- A storage pool is the aggregate storage capacity available for the provider to partition and assign to individual units. -->

A **storage pool** is a mechanism for administrators to define sources of storage that they will use to satisfy application storage requirements.

A single pool might be used for storage from units of many different applications - it is a resource from which different stores may be drawn.

A pool describes {ref}`storage provider <storage-provider>`-specific parameters for creating storage, such as performance (e.g. IOPS), media type (e.g. magnetic vs. SSD), or durability.

For many providers, there will be a shared resource where storage can be requested (e.g. for Amazon EC2, `ebs`). Creating pools there maps provider specific settings into named resources that can be used during deployment.

Pools defined at the model level are easily reused across applications. Pool creation requires a pool name, the provider type and attributes for configuration as space-separated pairs, e.g. tags, size, path, etc.


(storage-provider)=
## Storage provider

In Juju, a **storage provider** refers to the technology used to make storage available to a charm.

### List of storage providers

There are three storage providers you can use with all clouds: `loop`, `rootfs`, and `tmpfs`. In addition, for some clouds there are also cloud-specific providers.

#### `<cloud-specific storage provider>`

See {ref}`list-of-supported-clouds` > `<cloud name>` > Storage providers.


#### `loop`
```{ibnote}
See also: [Wikipedia | Loop device](https://en.wikipedia.org/wiki/Loop_device)
```

Block-type. Creates a file on the unit's root filesystem, associates a loop device with it. The loop device is provided to the charm.

```{note}
Loop devices require extra configuration to be used within LXD. See more: {ref}`storage-provider-lxd`.
```

#### `rootfs`
```{ibnote}
See also: [The Linux Kernel Archives | ramfs, rootfs and initramfs](https://www.kernel.org/doc/Documentation/filesystems/ramfs-rootfs-initramfs.txt)
```

Filesystem-type. Creates a sub-directory on the unit's root filesystem for the unit/charmed operator to use.

#### `tmpfs`
```{ibnote}
See also: [Wikipedia | Tmpfs](https://en.wikipedia.org/wiki/Tmpfs)
```

Filesystem-type. Creates a temporary file storage facility that appears as a mounted file system but is stored in volatile memory.




