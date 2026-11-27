---
myst:
  html_meta:
    description: "Juju storage reference: data volumes, storage directives, pools, providers, and dynamic storage management across clouds."
---

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

The order of the arguments does not actually matter -- they are identified based on a regex (sizes must end with a unit suffix; for pool names, see {ref}`storage-pool`).

If at least one storage directive component is specified, the following default values come into effect:

* `<pool>`: the default storage pool.
* `<count>`: the minimum number required by the charm, or '1' if the storage is optional
* `<size>`: determined from the charm's minimum storage size, or 1GiB if the charm does not specify a minimum

In the absence of any explicit storage directive, the storage will be put on the root filesystem (`rootfs`).

## Storage pool

A storage directive names a storage pool to say where an application's storage comes from. For what a pool is, and what Juju accepts and refuses when pools are created, updated, removed and listed, see {ref}`storage-pool`.

(storage-provider)=
## Storage provider

In Juju, a **storage provider** refers to the technology used to make storage available to a charm.

### List of storage providers

There are three storage providers you can use with all clouds: `loop`, `rootfs`, and `tmpfs`. In addition, for some clouds there are also cloud-specific providers.

(storage-provider-cloud-specific)=
#### Cloud-specific storage providers

Many clouds provide additional storage providers beyond the generic ones. For the cloud-specific storage providers available on your cloud, see {ref}`list-of-supported-clouds` > `<cloud name>` > Storage.

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

