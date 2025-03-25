(hook-command-storage-add)=
# `storage-add`

## Summary
Add storage instances.

## Usage
``` storage-add [options] <charm storage name>[=count] ...```

## Examples

    storage-add database-storage=1


## Details

Storage add adds storage instances to unit using provided storage directives.
A storage directive consists of a storage name as per charm specification
and optional storage COUNT.

COUNT is a positive integer indicating how many instances
of the storage to create. If unspecified, COUNT defaults to 1.

Further details:

storage-add adds storage volumes to the unit.
storage-add takes the name of the storage volume (as defined in the
charm metadata), and optionally the number of storage instances to add.
By default, it will add a single storage instance of the name.