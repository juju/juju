(hook-command-relation-model-get)=
# `relation-model-get`

## Summary
Get details about the model hosing a related application.

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--format` | smart | Specify output format (json&#x7c;smart&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |
| `-r`, `--relation` |  | Specify a relation by id |

## Details

-r must be specified when not in a relation hook

relation-model-get outputs details about the model hosting the application
on the other end of a unit relation.
If not running in a relation hook context, -r needs to be specified with a
relation identifier similar to the relation-get and relation-set commands.