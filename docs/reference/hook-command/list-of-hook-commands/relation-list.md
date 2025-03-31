(hook-command-relation-list)=
# `relation-list`
## Summary
List relation units.

## Usage
``` relation-list [options] ```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--app` | false | List remote application instead of participating units |
| `--format` | smart | Specify output format (json&#x7c;smart&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |
| `-r`, `--relation` |  | Specify a relation by id |

## Details

-r must be specified when not in a relation hook

relation-list outputs a list of all the related units for a relation identifier.
If not running in a relation hook context, -r needs to be specified with a
relation identifier similar to the relation-get and relation-set commands.