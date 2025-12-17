(command-juju-storage)=
# `juju storage`
> See also: [show-storage](#show-storage), [add-storage](#add-storage), [remove-storage](#remove-storage)

**Aliases:** list-storage

## Summary
Lists storage details.

## Usage
```juju storage [options] <filesystem|volume> ...```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Specifies whether to skip web browser for authentication. |
| `--filesystem` | false | (DEPRECATED) Specifies whether to list filesystem storage. |
| `--format` | tabular | Specify output format (json&#x7c;tabular&#x7c;yaml) |
| `-m`, `--model` |  | Specifies the model to operate in. Accepts `[<controller name>:]<model name>|<model UUID>`. |
| `-o`, `--output` |  | Specify an output file |
| `--volume` | false | (DEPRECATED) Specifies whether to list volume storage. |

## Examples

List all storage:

    juju storage

List only filesystem storage:

    juju storage --filesystem

List only volume storage:

    juju storage --volume


## Details

Lists information about storage.