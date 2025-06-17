(storage)=
# Storage

> See also: {ref}`manage-storage`

In Juju, **storage** refers to a data volume that is provided by a {ref}`cloud <cloud>`.

Depending on how things are set up during deployment, the data volume can be machine-dependent (e.g., a directory on disk tied to the machine which goes away if the unit is destroyed) or machine-independent (i.e., it can outlive a machine and be reattached to another machine).


(storage-constraint)=
## Storage constraint (directive)

In Juju, a **storage constraint** is a collection of storage specifications that can be passed as a positional argument to some commands (`add-storage`) or as the argument to the `--storage` option of other commands (`deploy`, `refresh`) to dictate how storage is allocated.

<!--When you perform `juju deploy` or `juju add-storage`, you can dictate how storage is allocated by specifying a `--storage` option.-->

```{important}

A *storage constraint* is slightly different from a {ref}`constraint <constraint>` -- while the general meaning is similar, the syntax is quite different. For this reason, a storage constraint is sometimes also called a *storage directive*.

```

```{important}

To put together a storage constraint, you need information from both the charm and the {ref}`storage provider <storage-provider>` /  {ref}`storage pool <storage-pool>`.

```

This constraint has the form `<label>=<pool>,<count>,<size>`.

```{important}

The order of the arguments does not actually matter -- they are identified based on a regex (pool names must start with a letter and sizes must end with a unit suffix).

```

<!-- Where in the charm project exactly do you find this label?-->

`<label>` is a string taken from the charmed operator itself. It encapsulates a specific storage option/feature. Sometimes it is also called a *store*.

The values are as follows:

*   `<pool>`: the storage pool. See more: {ref}`storage-pool`.
*   `<count>`: number of volumes
*   `<size>`: size of each volume


If at least one constraint is specified the following default values come into effect:

* `<pool>`: the default storage pool. See more: {ref}`manage-storage-pools`.
* `<count>`: the minimum number required by the charm, or '1' if the storage is optional
* `<size>`: determined from the charm's minimum storage size, or 1GiB if the charmed operator does not specify a minimum


````{dropdown} Expand to see an example of a partial specification and its fully-specified equivalent


Suppose you want to deploy PostgreSQL with one instance (count) of 100GiB, via the charm's 'pgdata' storage label, using the default storage pool:

```text
juju deploy postgresql --storage pgdata=100G
```

Assuming an AWS model, where the default storage pool is `ebs`, this is equivalent to:

```text
juju deploy postgresql --storage pgdata=ebs,100G,1
```

````


In the absence of any storage specification, the storage will be put on the root filesystem (`rootfs`). <!--I'm guessing this takes care of `<pool>`. What about the other values?-->

 `--storage` may be specified multiple times, to support multiple charm labels.


(storage-pool)=
## Storage pool
> See also: {ref}`manage-storage-pools`


```{important}
A storage pool is defined on top of a {ref}`storage provider <storage-provider>`.
```

<!--
A storage pool is the aggregate storage capacity available for the provider to partition and assign to individual units.
-->

<!--from https://discourse.charmhub.io/t/juju-create-storage-pool/1702 >> Details: -->

A **storage pool** is a mechanism for administrators to define sources of storage that they will use to satisfy application storage requirements.

A single pool might be used for storage from units of many different applications - it is a resource from which different stores may be drawn.

A pool describes provider-specific parameters for creating storage, such as performance (e.g. IOPS), media type (e.g. magnetic vs. SSD), or durability.

For many providers, there will be a shared resource where storage can be requested (e.g. for Amazon EC2, `ebs`). Creating pools there maps provider specific settings into named resources that can be used during deployment.

Pools defined at the model level are easily reused across applications. Pool creation requires a pool name, the provider type and attributes for configuration as space-separated pairs, e.g. tags, size, path, etc.

For Kubernetes models, the provider type defaults to “kubernetes” unless otherwise specified.


(storage-provider)=
## Storage provider

In Juju, a **storage provider** refers to the technology used to make storage available to a charm.



### List of generic storage providers

There are several cloud-independent storage providers, which are available to all types of models:

#### `loop`
> See also: [Wikipedia | Loop device](https://en.wikipedia.org/wiki/Loop_device)

*    Block-type, creates a file on the unit's root filesystem, associates a loop device with it. The loop device is provided to the charm.

#### `rootfs`
> See also: [The Linux Kernel Archives | ramfs, rootfs and initramfs](https://www.kernel.org/doc/Documentation/filesystems/ramfs-rootfs-initramfs.txt)

*   Filesystem-type, creates a sub-directory on the unit's root filesystem for the unit/charmed operator to use. Works with Kubernetes models.

#### `tmpfs`
> See also: [Wikipedia | Tmpfs](https://en.wikipedia.org/wiki/Tmpfs)

*   Filesystem-type, creates a temporary file storage facility that appears as a mounted file system but is stored in volatile memory. Works with Kubernetes models.

```{note}

Loop devices require extra configuration to be used within LXD. For that, please refer to {ref}`loop-devices-and-lxd`.

```
<!--
Providers 'rootfs' and 'tmpfs' are used, respectively, to map storage of a virtualisation host to the root disk or to an in-memory filesystem.
-->

### List of cloud-specific storage providers

(storage-provider-azure)=
#### `azure`

Azure-based models have access to the 'azure' storage provider.

The 'azure' storage provider has an 'account-type' configuration option that accepts one of two values: 'Standard_LRS' and 'Premium_LRS'. These are, respectively, associated with defined Juju pools 'azure' and 'azure-premium'.

Newly-created models configured in this way use "Azure Managed Disks". See [Azure Managed Disks Overview](https://docs.microsoft.com/en-us/azure/virtual-machines/windows/managed-disks-overview) for information on what this entails (in particular, what the difference is between standard and premium disk types).

(storage-provider-cinder)=
#### `cinder`

OpenStack-based models have access to the 'cinder' storage provider.

The 'cinder' storage provider has a 'volume-type' configuration option whose value is the name of any volume type registered with Cinder.

(storage-provider-ebs)=
#### `ebs`

AWS-based models have access to the 'ebs' storage provider, which supports the following pool attributes:

**volume-type**

* Specifies the EBS volume type to create. You can use either the EBS volume type names, or synonyms defined by Juju (in parentheses):

    *   standard (magnetic)
    *   gp2 (ssd)
    *   gp3
    *   io1 (provisioned-iops)
    *   io2
    *   st1 (optimized-hdd)
    *   sc1 (cold-storage)

    Juju's default pool (also called 'ebs') uses gp2/ssd as its own default.

**iops**

* The number of IOPS for io1, io2 and gp3 volume types. There are restrictions on minimum and maximum IOPS, as a ratio of the size of volumes. See [Provisioned IOPS (SSD) Volumes](https://docs.aws.amazon.com/ebs/latest/userguide/provisioned-iops.html) for more information.

**encrypted**

* Boolean (true|false); indicates whether created volumes are encrypted.

**kms-key-id**

* The KMS Key ARN used to encrypt the disk. Requires *encrypted: true* to function.

**throughput**

* The number of megabyte/s throughput a GP3 volume is provisioned for. Values are passed in the form *1000M* or *1G* etc.

```{note}

For detailed information regarding EBS volume types, see the [AWS EBS documentation](http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/EBSVolumeTypes.html).

```
(storage-provider-gce)=
#### `gce`

Google-based models have access to the 'gce' storage provider. The GCE provider does not currently have any specific configuration options.

(storage-provider-kubernetes)=
#### `kubernetes`
> See also: [Persistent storage and Kubernetes](https://discourse.charmhub.io/t/topic/1078)

Kubernetes-based models have access to the 'kubernetes' storage provider, which supports the following pool attributes:

**storage-class**

*   The storage class for the Kubernetes cluster to use. It can be any storage class that you have defined, for example:

    *   `juju-unit-storage`
    *   `juju-charm-storage`
    *   `microk8s-hostpath`

**storage-provisioner**

*   The Kubernetes storage provisioner. For example:

    *   `kubernetes.io/no-provisioner`
    *   `kubernetes.io/aws-ebs`
    *   `kubernetes.io/gce-pd`

**parameters.type**

*   Extra parameters. For example:

    *   `gp2`
    *   `pd-standard`


(storage-provider-lxd)=
#### `lxd`

```{note}

The regular package archives for Ubuntu 14.04 LTS (Trusty) and Ubuntu 16.04 LTS (Xenial) do not include a version of LXD that has the 'lxd' storage provider feature. You will need at least version 2.16. See {ref}`cloud-lxd` for installation help.

```

LXD-based models have access to the 'lxd' storage provider. The LXD provider has two configuration options:

**driver**

*    This is the LXD storage driver (e.g. zfs, btrfs, lvm, ceph).

**lxd-pool**

*    The name to give to the corresponding storage pool in LXD.

Any other parameters will be passed to LXD (e.g. zfs.pool_name). See upstream [LXD storage configuration](https://github.com/lxc/lxd/blob/master/doc/storage.md) for LXD storage parameters.

Every LXD-based model comes with a minimum of one LXD-specific Juju storage pool called 'lxd'. If ZFS and/or BTRFS are present when the controller is created then pools 'lxd-zfs' and/or 'lxd-btrfs' will also be available. The following output to the `juju storage-pools` command shows all three Juju LXD-specific pools:

```bash
Name       Provider  Attributes
loop       loop
lxd        lxd
lxd-btrfs  lxd       driver=btrfs lxd-pool=juju-btrfs
lxd-zfs    lxd       driver=zfs lxd-pool=juju-zfs zfs.pool_name=juju-lxd
rootfs     rootfs
tmpfs      tmpfs
```

As can be inferred from the above output, for each Juju storage pool based on the 'lxd' storage provider there is a LXD storage pool that gets created. It is these LXD pools that will house the actual volumes.

The LXD pool corresponding to the Juju 'lxd' pool doesn't get created until the latter is used for the first time (typically via the `juju deploy` command). It is called simply 'juju'.

The command `lxc storage list` is used to list LXD storage pools. A full "contingent" of LXD non-custom storage pools would like like this:

```bash
+------------+-------------+--------+------------------------------------+---------+
|    NAME    | DESCRIPTION | DRIVER |               SOURCE               | USED BY |
+------------+-------------+--------+------------------------------------+---------+
| default    |             | dir    | /var/lib/lxd/storage-pools/default | 1       |
+------------+-------------+--------+------------------------------------+---------+
| juju       |             | dir    | /var/lib/lxd/storage-pools/juju    | 0       |
+------------+-------------+--------+------------------------------------+---------+
| juju-btrfs |             | btrfs  | /var/lib/lxd/disks/juju-btrfs.img  | 0       |
+------------+-------------+--------+------------------------------------+---------+
| juju-zfs   |             | zfs    | /var/lib/lxd/disks/juju-zfs.img    | 0       |
+------------+-------------+--------+------------------------------------+---------+
```

The three Juju-related pools above are for storing *volumes* that Juju applications can use. The fourth 'default' pool is the standard LXD storage pool where the actual *containers* (operating systems) live.

To deploy an application, refer to the pool as usual. Here we deploy PostgreSQL using the 'lxd' Juju storage pool, which, in turn, uses the 'juju' LXD storage pool:

```bash
juju deploy postgresql --storage pgdata=lxd,8G
```

See {ref}`cloud-lxd` for how to use LXD in conjunction with Juju, including the use of ZFS as an alternative filesystem.


(loop-devices-and-lxd)=
##### Loop devices and LXD

LXD (localhost) does not officially support attaching loopback devices for storage out of the box. However, with some configuration you can make this work.

Each container uses the 'default' LXD profile, but also uses a model-specific profile with the name `juju-<model-name>`. Editing a profile will affect all of the containers using it, so you can add loop devices to all LXD containers by editing the 'default' profile, or you can scope it to a model.

To add loop devices to your container, add entries to the 'default', or model-specific, profile, with `lxc profile edit <profile>`:

``` yaml
...
devices:
  loop-control:
    major: "10"
    minor: "237"
    path: /dev/loop-control
    type: unix-char
  loop0:
    major: "7"
    minor: "0"
    path: /dev/loop0
    type: unix-block
  loop1:
    major: "7"
    minor: "1"
    path: /dev/loop1
    type: unix-block
...
  loop9:
    major: "7"
    minor: "9"
    path: /dev/loop9
    type: unix-block
```

Doing so will expose the loop devices so the container can acquire them via the `losetup` command. However, it is not sufficient to enable the container to mount filesystems onto the loop devices. One way to achieve that is to make the container "privileged" by adding:

``` yaml
config:
  security.privileged: "true"
```

### `maas`

MAAS has support for discovering information about machine disks, and an API for acquiring nodes with specified disk parameters. Juju's MAAS provider has an integrated 'maas' storage provider. This storage provider is static-only; it is only possible to deploy charmed operators using 'maas' storage to a new machine in MAAS, and not to an existing machine, as described in the section on dynamic storage.

The MAAS provider currently has a single configuration attribute:

**tags**

* A comma-separated list of tags to match on the disks in MAAS. For example, you might tag some disks as 'fast'; you can then create a storage pool in Juju that will draw from the disks with those tags.



### `oracle`

Oracle-based models have access to the 'oracle' storage provider. The Oracle provider currently supports a single pool configuration attribute:

**volume-type**

*   Volume type, a value of 'default' or 'latency'. Use 'latency' for low-latency, high IOPS requirements, and 'default' otherwise.

    For convenience, the Oracle provider registers two predefined pools:

    *   'oracle' (volume type is 'default')
    *   'oracle-latency' (volume type is 'latency').



(dynamic-storage)=
## Dynamic storage


Most storage can be dynamically added to, and removed from, a unit. Some types of storage, however, cannot be dynamically managed. For instance, Juju cannot disassociate MAAS disks from their respective MAAS nodes. These types of static storage can only be requested at deployment time and will be removed when the machine is removed from the model.

Certain cloud providers may also impose restrictions when attaching storage. For example, attaching an EBS volume to an EC2 instance requires that they both reside within the same availability zone. If this is not the case, Juju will return an error.

When deploying an application or unit that requires storage, using machine placement (i.e. `--to`) requires that the assigned storage be dynamic. Juju will return an error if you try to deploy a unit to an existing machine, while also attempting to allocate static storage.



<!--Charmed operators can be made to be storage-aware, allowing data storage that persists beyond the lifetime of any given machine.-->


<!--
It's not always "machine independent" data volume. It can be but can also just be a directory on disk tied to the machine
Storage declares that a charm needs either a block or filesystem - how that is realised is determined at deployment. For development, a filesystem storage requirement might be a directory on disk under the charm dir and hence is tied to the machine and goes away when the unit is destroyed. Or it could be (like the text currently says) a machine independent volume which can (if the user so chooses) outlive the machine to which it was attached and be reattached later elsewhere. It's hard to define everything n just one sentence as it's a complex topic-->



## Defining storage
<!--TODO incorporate any useful detail into https://juju.is/docs/sdk/use-storage-in-a-charm, then delete this section.-->

### In general

Charm storage is defined in the [`storage` key in `charmcraft.yaml`](https://juju.is/docs/sdk/charmcraft-yaml#heading--storage).

The `storage` map definition:

|     Field      |            Type             | Default  | Description                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| :------------: | :-------------------------: | :------: | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
|     `type`     |          `string`           | required | Type of storage requested. Supported values are `block` or `filesystem`.<br/><br/>The `filesystem` type yields a directory in which the charm may store files. The `block` type yields a raw block device, typically disks or logical volumes.<br/><br/>If the charm specifies a `filesystem`-type store, and the storage provider supports provisioning only disks, then a disk will be created, attached, partitioned, and a filesystem created on top. The `filesystem` will be presented to the charm as normal. |
| `description`  |          `string`           |  `nil`   | Description of the storage requested                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
|   `multiple`   | `map`<br/>(see table below) |  `nil`   | By default, stores are singletons; a charm will have exactly one of the specified stores. The `multiple` field specifies the number of storage instances to be requested.<br/><br/>Unless a number is explicitly specified during deployment, units of the application will be allocated the minimum number of storage instances specified in the charm metadata. It is then possible to add instances (up to the maximum) by using the `juju storage add command`.                                                  |
| `minimum-size` |          `string`           |  `1GiB`  | Size in the forms: `1.0G`, `1GiB`, `1.0GB`. Supported size multipliers are `M`, `G`, `T`, `P`, `E`, `Z`, `Y`. Not specifying a multiplier implies `M`.                                                                                                                                                                                                                                                                                                                                                               |
|   `location`   |          `string`           |  `nil`   | Specifies the mount location for `filesystem` stores. For multi-stores, the location acts as the parent directory for each mounted store.                                                                                                                                                                                                                                                                                                                                                                            |
|  `properties`  |         `string[]`          |  `nil`   | List of properties for the storage. Currently only `transient` is supported                                                                                                                                                                                                                                                                                                                                                                                                                                          |
|  `shared`  |         `bool`          |  `false`   | True indicates that all units of the application share the storage.                                                                                                                                                                                                                                                                                                                                                                                                                                          |

The `multiple` map definition:

|  Field  |      Type      | Default | Description                                                                                                                                                                      |
| :-----: | :------------: | :-----: | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `range` | `string`/`int` |   nil   | Value can be an `int` for a precise number, or a `string` in the forms: `m-n`, `m+`, `m-`, where `m` and `n` are of type `int`. <br/><br/>Examples: `range: 2` or `range: 0-10`. |

An example of a storage definition inside `metadata.yaml`:

```yaml
# ...
storage:
  # Name of this storage is 'data'
  data:
    type: filesystem
    description: junk storage
    minimum-size: 100M
    location: /srv/data
# ...
```

### On Kubernetes

In addition to the above, there is some additional data required to define storage for Kubernetes charms. You will still need to define the top-level storage map (as above), but also specify which containers you would like the storage mounted into. Consider the following `metadata.yaml` snippet:

```yaml
# ...
containers:
  # define a container named "important-app"
  important-app:
    # use the "app-image" oci resource
    resource: app-image
    # mount our 'logs' store at /var/log/important-app
    # in the workload container
    mounts:
      - storage: logs
        location: /var/log/important-app
  # This is another container with no storage
  supporting-app:
    resource: supporting-app-image

storage:
  logs:
    type: filesystem
    # specifying location on the charm container is optional
    # when unspecified, defaults to /var/lib/juju/storage/<name>/<num>
# ...
```

The above snippet will ensure that both the `important-app` container and charm container inside each Pod has the `logs` store mounted. Under the hood, the `storage` map is translated into a series of [`PersistentVolume`](https://kubernetes.io/docs/concepts/storage/persistent-volumes/)s, mounted into Pods with [`PersistentVolumeClaim`](https://kubernetes.io/docs/concepts/storage/persistent-volumes/)s.

```{note}

The `location` attribute *must* be specified when mounting a storage into a workload container as shown above - this will dictate the mount point for the specific container.

Optionally, developers can specify the `location` attribute on the storage itself, which will specify the mount point in the charm container. If left unset, the charm container will have the storage volume mounted at a predictable path at `/var/lib/juju/storage/<name>/<num>`, where `<num>` is the index of the storage. This defaults to `0`.

For the above `metadata.yaml`, the charm container would have the storage available at: `/var/lib/juju/storage/logs/0`.

```


## Storage events


There are two key events associated with storage:

|         Event name         |                                                 Event Type                                                 | Description                                                                                                                                                                                                                                                                                                                                                                                                                     |
| :------------------------: | :--------------------------------------------------------------------------------------------------------: | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `<name>_storage_attached`  | [`StorageAttachedEvents`](https://ops.readthedocs.io/en/latest/index.html#ops.StorageAttachedEvent)  | This event is triggered when new storage is available for the charm to use. Callback methods bound to this event allow the charm to run code when storage has been added.<br/><br/>Such methods will be run before the `install` event fires, so that the installation routine may use the storage. The name prefix of this hook will depend on the storage key defined in the `metadata.yaml` file.                            |
| `<name>_storage_detaching` | [`StorageDetachingEvent`](https://ops.readthedocs.io/en/latest/index.html#ops.StorageDetachingEvent) | Callback methods bound to this event allow the charm to run code before storage is removed.<br/><br/>Such methods will be run before storage is detached, and always before the `stop` event fires, thereby allowing the charm to gracefully release resources before they are removed and before the unit terminates.<br/><br/>The name prefix of the hook will depend on the storage key defined in the `metadata.yaml` file. |

### Charm and container access to storage

When you use storage mounts with juju, it will be automatically mounted into the charm container
at either:

* the specified `location` based on the storage section of metadata.yaml or

* the default location `/var/lib/juju/storage/<storage-name>/<num>` where `num`
  is zero for "normal"/singular storages or integer id for storages that support `multiple`
  attachments.

The operator framework provides the `Model.storages` dict-like member that maps storage names to a
list of storages mounted under that name.  It is a list in order to handle the case of storage
configured for multiple instances.  For the basic singular case, you will simply access the
first/only element of this list.

Charm developers should *not* directly assume a location/path for mounted storage.  To access
mounted storage resources, retrieve the desired storage's mount location from within your charm
code - e.g.:

```python
def _my_hook_function(self, event):
    ...
    storage = self.model.storages['my-storage'][0]
    root = storage.location

    fname = 'foo.txt'
    fpath = os.path.join(root, fname)
    with open(fpath, 'w') as f:
        f.write('super important config info')
    ...
```

This example utilizes the framework's representation of juju storage - i.e. `self.model.storages`
which returns a [`mapping`](https://ops.readthedocs.io/en/latest/index.html#ops.StorageMapping) of
`<storage_name>` to [`Storage`](https://ops.readthedocs.io/en/latest/index.html#ops.Storage)
objects, which exposes the `name`, `id` and `location` of each storage to the charm developer,
where `id` is the underlying storage provider ID.

If you have also mounted storage in a container, that storage will be located directly at the
specified mount location.  For example with the following content in your metadata.yaml:

```
containers:
  foo:
    resource: foo-image
    mounts:
      - storage: data
        location: /foo-data
```

storage for the "foo" container will be mounted directly at `/foo-data`.  There are no storage name
or integer-indexed subdirectories. Juju does not currently support multiple storage instances for
charms using "containers" functionality.  If you are writing a container-based charm (e.g. for
kubernetes clouds) it is best to have your charm code communicate the storage location to the
workload rather than hard-coding the storage path in the container itself.  This can be
accomplished by various means. One method is passing the mount path via a file using the
`Container` API:

```python
def _on_mystorage_storage_attached(self, event):
    # get the mount path from the charm metadata
    container_meta = self.framework.meta.containers['my-container']
    storage_path = container_meta.mounts['my-storage'].location
    # push the path to the workload container
    c = self.model.unit.get_container('my-container')
    c.push('/my-app-config/storage-path.cfg', storage_path)

    ... # tell workload service to reload config/restart, etc.
```

### Scaling storage

While juju provides an `add-storage` command, this does not "grow" existing storage
instances/mounts like you might expect.  Rather it works by increasing the number of storage
instances available/mounted for storages configured with the `multiple` parameter.  For charm
development, handling storage scaling (add/detach) amounts to handling `<name>_storage_attached`
and `<name_storage_detaching` events. For example, with the following in your metadata.yaml file:

```yaml
storage:
    my-storage:
        type: filesystem
        multiple:
            range: 1-10
```

juju will deploy the application with the minimum of the range (1 storage instance in the example
above).  Storage with this type of `multiple:...` configuration will have each instance residing
under an indexed subdirectory of that storage's main directory - e.g.
`/var/lib/juju/storage/my-storage/1` by default in charm container.  Running `juju add-storage
<unit> my-storage=32G,2` will add two additional instances to this storage - e.g.:
`/var/lib/juju/storage/my-storage/2` and `/var/lib/juju/storage/my-storage/3`.  "Adding" storage
does not modify or affect existing storage mounts.  This would generate two separate
storage-attached events that should be handled.

In addition to juju client requests for adding storage, the [`StorageMapping`](https://ops.readthedocs.io/en/latest/index.html#ops.StorageMapping)
returned by `self.model.storages` also exposes a
[`request`](https://ops.readthedocs.io/en/latest/index.html#ops.StorageMapping.request)
method (e.g. `self.model.storages.request()`) which provides an expedient method for the developer
to invoke the underlying {ref}`storage-add <hook-command-storage-add>` hook command in
the charm to request additional storage. On success, this will fire a
{ref}`storage-attached <hook-storage-attached>` hook.



(storage-support)=
## Storage Support

### Persistent storage for stateful applications

#### Example

On AWS, an example using dynamic persistent volumes.
```text
juju bootstrap aws
juju deploy kubernetes-core
juju deploy aws-integrator
juju trust aws-integrator
```

**Note**: the aws-integrator charm is needed to allow dynamic storage provisioning.

Wait for `juju status` to go green.
```text
juju scp kubernetes-master/0:config ~/.kube/config
juju add-k8s myk8scloud
juju add-model myk8smodel myk8scloud
juju create-storage-pool k8s-ebs kubernetes storage-class=juju-ebs storage-provisioner=kubernetes.io/aws-ebs parameters.type=gp2
juju deploy cs:~wallyworld/mariadb-k8s --storage database=10M,k8s-ebs
```
Now you can see the storage being created/attached using `juju storage`.

`juju storage`
or
`juju storage --filesystem`
or
`juju storage --volume`
or
`juju storage --format yaml`

You can also see the persistent volumes and volume claims being created in Kubernetes.

`kubectl -n myk8smodel get all,pvc,pv`


#### In more detail

Application pods may be restarted, either by Juju to perform an upgrade, or at the whim of Kubernetes itself. Applications like databases which require persistent storage can make use of Kubernetes persistent volumes.

As with any other charm, Kubernetes charms may declare that storage is required. This is done in metadata.yaml.

```yaml
storage:
  database:
    type: filesystem
    location: /var/lib/mysql
```

An example charm is [mariadb-k8s](https://jujucharms.com/u/juju/mariadb-k8s).

Only filesystem storage is supported at the moment. Block volume support may come later.

There's 2 ways to configure the Kubernetes cluster to provide persistent storage:

1. A pool of manually provisioned, static persistent volumes

2. Using a storage class for dynamic provisioning of volumes

In both cases, you use a Juju storage pool and can configure it to supply extra Kubernetes specific configuration if needed.

#### Manual persistent volumes

This approach is mainly intended for testing/prototyping.

You can create persistent volumes using whatever backing provider is supported by the underlying cloud. One or many volumes may be created. The _storageClassName_ attribute of each volume needs to be set to an arbitrary name.

Next create a  storage pool in Juju which will allow the use of the persistent volumes:

`
juju create-storage-pool <poolname> kubernetes storage-class=<classname> storage-provisioner=kubernetes.io/no-provisioner
`

_classname_ is the base storage class name assigned to each volume.
_poolname_ will be used when deploying the charm.


Kubernetes will pick an available available volume each time it needs to provide storage to a new pod. Once a volume is used, it is never re-used, even if the unit/pod is terminated and the volume is released. Just as volumes are manually created, they must also be manually deleted.

This approach is useful for testing/protyping. If you deploy the kubernetes-core bundle, you can create one or more "host path" persistent volumes on the worker node (each mounted to a different directory). Here's an example YAML config file to use with kubectl to create 1 volume:

```
kind: PersistentVolume
apiVersion: v1
metadata:
  name: mariadb-data
spec:
  capacity:
    storage: 100Mi
  accessModes:
    - ReadWriteOnce
  persistentVolumeReclaimPolicy: Retain
  storageClassName: <model>-mariadb-unit-storage
  hostPath:
    path: "/mnt/data"
```
You'd tweak the host path and volume name to create a selection of persistent volumes to test with - remember, each manually created volume can only be used once.

**Note:** the storage class name in the PV YAML above has the model name prepended to it. This is because storage classes are global to the cluster and so Juju will prepend the model name to disambiguate. So you will need to know the model name when setting up static PVs. Or you can create them and edit the storage class attribute later using `kubectl edit`.

Then create the Juju storage pool:

```text
juju create-storage-pool test-storage kubernetes storage-class=mariadb-unit-storage storage-provisioner=kubernetes.io/no-provisioner
```

Now deploy the charm:

```text
juju deploy cs:~juju/mariadb-k8s --storage database=10M,test-storage
```

Juju will create a suitably named Kubernetes storage class with the relevant provisioner type to enable the use of the statically created volumes.

#### Dynamic Persistent Volumes

To allow for Kubernetes to create persistent volumes on demand, a Kubernetes storage class is used. This is done automatically by Juju if you create a storage pool. As with vm based clouds, a Juju storage pool configures different classes of storage which are available to use with deployed charm.

It's also possible to set up a Kubernetes storage class manually and have finer grained control over how things tie together, but that's beyond the scope of this topic.

Before deploying your charm which requires storage, create a Juju storage pool which defines what backing provider will be used to provision the dynamic persistent volumes. The backing provider is specific to the underlying cloud and more details are available in the Kubernetes [storage class](https://kubernetes.io/docs/concepts/storage/storage-classes/) documentation.

The example below is for a Kubernetes cluster deployed on AWS requiring EBS persistent volumes of type gp2. The name of the pool is arbitrary - in this case _k8s-eb2_. Note that the Kubernetes cluster needs be deployed with the cloud specific integrator charm as described earlier.

`juju create-storage-pool k8s-ebs kubernetes storage-class=juju-ebs storage-provisioner=kubernetes.io/aws-ebs parameters.type=gp2`

You can see what storage pools have been set up in Juju.

`juju storage-pools`

**Note**: only pools of type "kubernetes" are currently supported. rootfs, tmpfs and loop are unsupported.

Once a storage pool is set up, to define how Kubernetes should be configured to provide dynamic volumes, you can go ahead a deploy a charm using the standard Juju storage directives.

`juju deploy cs:~juju/mariadb-k8s --storage database=10M,k8s-ebs`

Use `juju storage` command (and its variants) to see the state of the storage.

If you scale up

`juju scale-application mariadb 3`

you will see that 2 need EBS volumes are created and become attached.

If you scale down

`juju scale-application mariadb 2`

you will see that one of the EBS volumes becomes detached but is still associated wih the model.

Scaling up again

`juju scale-application mariadb 3`

will result in this detached storage being reused and attached to the new unit.

Destroying the entire model will result in all persistent volumes for that model also being deleted.

#### What's next?

https://discourse.jujucharms.com/t/advanced-storage-support/204
