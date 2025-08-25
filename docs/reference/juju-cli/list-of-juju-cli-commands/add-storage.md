(command-juju-add-storage)=
# `juju add-storage`
> See also: [import-filesystem](#import-filesystem), [storage](#storage), [storage-pools](#storage-pools)

## Summary
Adds storage to a unit after it has been deployed.

## Usage
```juju add-storage [options] <unit> <storage-directive>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |

## Examples

Add a 100MiB tmpfs storage instance for "pgdata" storage to unit postgresql/0:

    juju add-storage postgresql/0 pgdata=tmpfs,100M

Add 10 1TiB storage instances to "osd-devices" storage to unit ceph-osd/0 from the model's default storage pool:

    juju add-storage ceph-osd/0 osd-devices=1T,10

Add a storage instance from the (AWS-specific) ebs-ssd storage pool for "brick" storage to unit gluster/0:

    juju add-storage gluster/0 brick=ebs-ssd


Further reading:

https://juju.is/docs/storage


## Details

Add storage to a pre-existing unit within a model. Storage is allocated from
a storage pool, using parameters provided within a "storage directive". (Use
`juju deploy --storage=<storage-directive>` to provision storage during the
deployment process.)

	juju add-storage &lt;unit&gt; &lt;storage-directive&gt;

`<unit>` is the ID of a unit that is already in the model.

`<storage-directive>` describes to the charm how to refer to the storage,
and where to provision it from. `<storage-directive>` takes the following form:

    <storage-name>[=<storage-constraint>]

`<storage-name>` is defined in the charm's `metadata.yaml` file.

`<storage-constraint>` is a description of how Juju should provision storage
instances for the unit. They are made up of up to three parts: `<storage-pool>`,
`<count>`, and `<size>`. They can be provided in any order, but we recommend the
following:

    <storage-pool>,<count>,<size>

Each parameter is optional, so long as at least one is present. So the following
storage constraints are also valid:

    <storage-pool>,<size>
    <count>,<size>
    <size>

`<storage-pool>` is the storage pool to provision storage instances from. Must
be a name from `juju storage-pools`.  The default pool is available via
executing `juju model-config storage-default-block-source`.

`<count>` is the number of storage instances to provision from `<storage-pool>` of
`<size>`. Must be a positive integer. The default count is "1". May be restricted
by the charm, which can specify a maximum number of storage instances per unit.

`<size>` is the number of bytes to provision per storage instance. Must be a
positive number, followed by a size suffix.  Valid suffixes include M, G, T,
and P.  Defaults to "1024M", or the which can specify a minimum size required
by the charm.