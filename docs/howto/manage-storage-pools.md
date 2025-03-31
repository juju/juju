(manage-storage-pools)=
# How to manage storage pools

> See also: {ref}`storage-pool`

This document shows how to work with storage pools. 

## Create a storage pool

First, check if your provider supports any storage configuration attributes. For example, in the case of AWS, the `ebs` storage provider supports several configuration attributes, and among these are `volume-type`, which configures the volume type (i.e. magnetic, ssd, or provisioned-iops), and `iops`, which indicates the IOPS per GiB.

> See more: {ref}`storage-provider` > `ebs`, [Wikipedia | IOPS](https://en.wikipedia.org/wiki/IOPS)

Second, use the `create-storage-pool` command, passing as parameters the desired name of the pool and the name of the provider and then all the key-value pairs that you want to specify. For example, the code below creates a storage pool with the name `iops` which is a version of `ebs` with 30 IOPS.

```text
juju create-storage-pool iops ebs volume-type=provisioned-iops iops=30
```

> See more: {ref}`command-juju-create-storage-pool`


## View the available storage pools

To view the available storage pools, use the `storage-pools` command:

```text
juju storage-pools
```

This will list all the predefined storage pools as well as any custom ones that may have been created with the `juju create-storage-pool` command.

```{note}

The name given to a default storage pool will often be the same as the name of the storage provider upon which it is based.

```

````{dropdown} Expand to view a sample output for a newly-added `aws` model

```text
Name     Provider  Attributes
ebs      ebs
ebs-ssd  ebs       volume-type=ssd
loop     loop
rootfs   rootfs
tmpfs    tmpfs
```

````

> See more: {ref}`command-juju-storage-pools`

## View the default storage pool

To find out the default storage pool for your block-type / filesystem-type, run the `model-config` command followed by the `storage-default-block-source` / `storage-default-filesystem-source` key. For example:

```text
juju model-config storage-default-block-source
```

> See more: {ref}`command-juju-model-config`, {ref}`storage-default-block-source`, {ref}`storage-default-filesystem-source` 


## Update a storage pool

To update storage pool attributes, use the `update-storage-pool` command:

```text
juju update-storage-pool test-pool
```

````{dropdown} Example


```text
# Update the storage-pool named iops with new configuration details:
juju update-storage-pool operator-storage volume-type=provisioned-iops iops=40

# Update which provider the pool is for:
juju update-storage-pool lxd-storage type=lxd-zfs
```

````

---

> See more: {ref}`command-juju-update-storage-pool`

## Remove a storage pool


To remove an existing storage pool, use the `remove-storage-pool` command:

```text
juju remove-storage-pool test-pool
```


> See more: {ref}`command-juju-remove-storage-pool`

