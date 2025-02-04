(hook-command-storage-get)=
# `storage-get`

## Summary
Print information for the storage instance with the specified ID.

## Usage
``` storage-get [options] [<key>]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--format` | smart | Specify output format (json&#x7c;smart&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |
| `-s` |  | specify a storage instance by id |

## Examples

    # retrieve information by UUID
    storage-get 21127934-8986-11e5-af63-feff819cdc9f

    # retrieve information by name
    storage-get -s data/0


## Details

When no &lt;key&gt; is supplied, all keys values are printed.

Further details:
storage-get obtains information about storage being attached
to, or detaching from, the unit.

If the executing hook is a storage hook, information about
the storage related to the hook will be reported; this may
be overridden by specifying the name of the storage as reported
by storage-list, and must be specified for non-storage hooks.

storage-get can be used to identify the storage location during
storage-attached and storage-detaching hooks. The exception to
this is when the charm specifies a static location for
singleton stores.