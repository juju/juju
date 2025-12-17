(command-juju-show-storage)=
# `juju show-storage`
> See also: [storage](#storage), [attach-storage](#attach-storage), [detach-storage](#detach-storage), [remove-storage](#remove-storage)

## Summary
Shows storage instance information.

## Usage
```juju show-storage [options] <storage ID> [...]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Specifies whether to skip web browser for authentication. |
| `--format` | yaml | Specify output format (json&#x7c;yaml) |
| `-m`, `--model` |  | Specifies the model to operate in. Accepts `[<controller name>:]<model name>|<model UUID>`. |
| `-o`, `--output` |  | Specify an output file |

## Examples

    juju show-storage storage-id


## Details

Shows extended information about storage instances.

Storage instances to display are specified by storage IDs.

Storage IDs are positional arguments to the command and do not need to be comma
separated when more than one ID is desired.