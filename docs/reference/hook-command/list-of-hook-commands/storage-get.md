(hook-command-storage-get)=
# `storage-get`
## Summary
Prints information for the storage instance with the specified ID.

## Usage
```text
storage-get [options] [-s <storage-id>] [<key>]
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--format` | smart | Specify output format (json&#x7c;smart&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |
| `-s` |  | Specifies a storage instance by ID. |

## Examples

Read the device path or mount point of the instance that triggered the current storage hook:

    storage-get location

Read the kind of storage instance data/0 after finding it with storage-list on this unit:

    storage-get -s data/0 kind


## Details

Inspect an available storage attachment of this unit. In a storage hook,
the default is the instance for that hook. Otherwise supply its Juju storage
ID with -s, using storage-list to find the IDs for this unit.

The optional key is kind or location; omit it to print both. A location is
a mount point for a filesystem or a device path for block storage.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/storage/