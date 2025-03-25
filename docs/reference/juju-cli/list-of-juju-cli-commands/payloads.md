(command-juju-payloads)=
# `juju payloads`
**Aliases:** list-payloads

## Summary
Display status information about known payloads.

## Usage
```juju payloads [options] [pattern ...]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--format` | tabular | Specify output format (json&#x7c;tabular&#x7c;yaml) |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `-o`, `--output` |  | Specify an output file |

## Details

This command will report on the runtime state of defined payloads.

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