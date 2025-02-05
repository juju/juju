(hook-command-action-get)=
# `action-get`
## Summary
Get action parameters.

## Usage
``` action-get [options] [<key>[.<key>.<key>...]]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--format` | smart | Specify output format (json&#x7c;smart&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |

## Examples

    TIMEOUT=$(action-get timeout)


## Details

action-get will print the value of the parameter at the given key, serialized
as YAML.  If multiple keys are passed, action-get will recurse into the param
map as needed.