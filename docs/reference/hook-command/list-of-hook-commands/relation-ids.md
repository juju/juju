(hook-command-relation-ids)=
# `relation-ids`
## Summary
List all relation IDs for the given endpoint.

## Usage
``` relation-ids [options] <name>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--format` | smart | Specify output format (json&#x7c;smart&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |

## Examples

    relation-ids database


## Details

relation-ids outputs a list of the related applications with a relation name.
Accepts a single argument (relation-name) which, in a relation hook, defaults
to the name of the current relation. The output is useful as input to the
relation-list, relation-get, relation-set, and relation-model-get commands
to read or write other relation values.

Only relation ids for relations which are not broken are included.