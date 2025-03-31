(hook-command-state-get)=
# `state-get`
> See also: [state-delete](#state-delete), [state-set](#state-set)

## Summary
Print server-side-state value.

## Usage
``` state-get [options] [<key>]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--format` | smart | Specify output format (json&#x7c;smart&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |
| `--strict` | false | Return an error if the requested key does not exist |

## Details

state-get prints the value of the server side state specified by key.
If no key is given, or if the key is "-", all keys and values will be printed.