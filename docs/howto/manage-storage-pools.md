---
myst:
  html_meta:
    description: "Create, configure, list, and manage storage pools in Juju. Customize storage attributes for AWS EBS, LXD, and other providers."
---

(manage-storage-pools)=
# How to manage storage pools

```{ibnote}
See also: {ref}`storage-pool`
```

This document shows how to work with storage pools.

(create-a-storage-pool)=
## Create a storage pool

```{ibnote}
See also: {ref}`Creating a storage pool <creating-a-storage-pool>`, {ref}`Storage pool names <storage-pool-names>`
```

First, check if your provider supports any storage configuration attributes. For example, in the case of AWS, the `ebs` storage provider supports several configuration attributes, and among these are `volume-type`, which configures the volume type (i.e. magnetic, ssd, or provisioned-iops), and `iops`, which indicates the IOPS per GiB.

```{ibnote}
See more: {ref}`storage-provider` > `ebs`, [Wikipedia | IOPS](https://en.wikipedia.org/wiki/IOPS)
```

To create an EBS pool on an AWS model, pass the pool name, the provider and the attributes to `juju create-storage-pool`. This pool requests 30 IOPS per GiB:

```text
juju create-storage-pool iops ebs volume-type=provisioned-iops iops=30
```

```{ibnote}
See more: {ref}`command-juju-create-storage-pool`
```

(view-the-available-storage-pools)=
## View the available storage pools

```{ibnote}
See also: {ref}`Listing storage pools <listing-storage-pools>`
```

To view the available storage pools, use the `storage-pools` command:

```text
juju storage-pools
```

````{dropdown} Expand to view a sample output for a newly-added aws model

```text
Name     Provider  Attributes
ebs      ebs
ebs-ssd  ebs       volume-type=ssd
loop     loop
rootfs   rootfs
tmpfs    tmpfs
```

````

To inspect a single pool's provider and attributes, give its name with `--name`. For example:

```text
juju storage-pools --name rootfs
```

```{ibnote}
See more: {ref}`command-juju-storage-pools`
```

(view-the-default-storage-pool)=
## View the default storage pool

```{ibnote}
See also: {ref}`default-storage-pools`
```

To find out the default storage pool for your block-type / filesystem-type, run the `model-config` command followed by the `storage-default-block-source` / `storage-default-filesystem-source` key. For example:

```text
juju model-config storage-default-block-source
```

```{ibnote}
See more: {ref}`command-juju-model-config`, {ref}`model-config-storage-default-block-source`, {ref}`model-config-storage-default-filesystem-source`
```

(update-a-storage-pool)=
## Update a storage pool

```{ibnote}
See also: {ref}`Updating a storage pool <updating-a-storage-pool>`
```

To update a storage pool:

1. Check the pool's current attributes:

   ```text
   juju storage-pools --name workload-storage
   ```

2. Run `update-storage-pool` with the pool's name and every attribute the pool should have after the update. To change the provider, add `type=<provider>`.

````{dropdown} Example

```text
# Replace the attributes of an EBS pool on an AWS model:
juju update-storage-pool workload-storage volume-type=provisioned-iops iops=40

# Change the provider of an AWS pool to EBS and set its attributes:
juju update-storage-pool workload-storage type=ebs volume-type=gp3
```

````

```{ibnote}
See more: {ref}`command-juju-update-storage-pool`
```

---

(remove-a-storage-pool)=
## Remove a storage pool

```{ibnote}
See also: {ref}`Removing a storage pool <removing-a-storage-pool>`
```

To remove a storage pool:

1. Check that the pool is not {ref}`in use <storage-pools-in-use>`. If the pool is a model default, change that default with {ref}`command-juju-model-config`.

2. Remove the pool:

   ```text
   juju remove-storage-pool test-pool
   ```

```{ibnote}
See more: {ref}`command-juju-remove-storage-pool`
```
