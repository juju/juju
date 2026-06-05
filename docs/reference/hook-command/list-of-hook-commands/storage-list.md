(hook-command-storage-list)=
# `storage-list`
## Summary
Lists storage attached to the unit.

## Usage
``` storage-list [options] [<storage-name>]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--format` | smart | Specify output format (json&#x7c;smart&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |

## Examples

    storage-list pgdata


## Details

`storage-list` will list the names of all storage instances
attached to the unit. These names can be passed to `storage-get`
via the `-s` flag to query the storage attributes.

A storage name may be specified, in which case only storage
instances for that named storage will be returned.