(hook-command-storage-add)=
# `storage-add`
## Summary
Adds storage instances.

## Usage
``` storage-add [options] <charm storage name>[=count] ...```

## Examples

    storage-add database-storage=1


## Details

`storage-add` adds storage volumes to the unit using the provided storage directives.

A storage directive consists of a storage name (as defined in the charm metadata)
and an optional storage count.

The count is a positive integer indicating how many instances of the storage to create.
If unspecified, the count defaults to 1.