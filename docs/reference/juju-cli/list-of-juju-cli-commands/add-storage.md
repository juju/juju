(command-juju-add-storage)=
# `juju add-storage`
> See also: [import-filesystem](#command-juju-import-filesystem), [storage](#command-juju-storage), [storage-pools](#command-juju-storage-pools)

## Summary
Adds storage to a unit after it has been deployed.

## Usage
```text
juju add-storage [options] <unit> <storage-directive> [<storage-directive> ...]
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |

## Examples

Request an additional 32 GiB EBS device if the Ceph OSD revision on ceph-osd/0
declares osd-devices and has room for one more instance:

    juju add-storage ceph-osd/0 osd-devices=ebs,32G,1

Request one additional OSD device for ceph-osd/0 using its current source and size:

    juju add-storage ceph-osd/0 osd-devices


## Details

Request additional instances of a storage name declared by the unit's charm
revision. The command reports the new storage IDs, not a completed attachment.
Inspect each ID with show-storage before using the storage.

Pass one or more directives as separate arguments. A bare storage name requests
one instance using the unit's current source and size. To override the request,
use name=pool,count,size without spaces. Components may be omitted or reordered;
use a positive integer for count and a size suffix such as G for GiB.
Existing and requested instances together must fit the charm's count range.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/storage/