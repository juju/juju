(command-juju-payloads)=
# `juju payloads`
**Aliases:** list-payloads

## Summary
Displays status information about known payloads.

## Usage
```juju payloads [options] [pattern ...]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Specifies whether to skip web browser for authentication. |
| `--format` | tabular | Specify output format (json&#x7c;tabular&#x7c;yaml) |
| `-m`, `--model` |  | Specifies the model to operate in. Accepts `[<controller name>:]<model name>|<model UUID>`. |
| `-o`, `--output` |  | Specify an output file |

## Details

Reports on the runtime state of defined payloads.

When one or more pattern is given, Juju will limit the results to only
those payloads which match *any* of the provided patterns. Each pattern
will be checked against the following info in Juju:

- unit name
- machine id
- payload type
- payload class
- payload id
- payload tag
- payload status