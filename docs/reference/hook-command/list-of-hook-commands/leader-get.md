(hook-command-leader-get)=
# `leader-get`
## Summary
Print application leadership settings.

## Usage
``` leader-get [options] [<key>]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--format` | smart | Specify output format (json&#x7c;smart&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |

## Examples

    ADDRESSS=$(leader-get cluster-leader-address)


## Details

leader-get prints the value of a leadership setting specified by key. If no key
is given, or if the key is "-", all keys and values will be printed.