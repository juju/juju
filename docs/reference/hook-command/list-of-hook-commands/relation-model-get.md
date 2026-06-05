(hook-command-relation-model-get)=
# `relation-model-get`
## Summary
Gets details about the model housing a related application.

## Usage
``` relation-model-get [options] ```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--format` | smart | Specify output format (json&#x7c;smart&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |
| `-r`, `--relation` |  | Specifies a relation by ID. |

## Details

`-r` must be specified when not in a relation hook.

`relation-model-get` outputs details about the model hosting the application
on the other end of a unit relation.
If not running in a relation hook context, `-r` needs to be specified with a
relation identifier similar to the `relation-get` and `relation-set` commands.