(command-juju-show-machine)=
# `juju show-machine`
> See also: [add-machine](#add-machine)

## Summary
Shows a machine's status.

## Usage
```juju show-machine [options] <machineID> ...```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Disables web browser for authentication. |
| `--color` | false | Forces use of ANSI color codes. |
| `--format` | yaml | Specify output format (json&#x7c;tabular&#x7c;yaml) |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `-o`, `--output` |  | Specify an output file |
| `--utc` | false | Displays time as UTC in RFC3339 format. |

## Examples

    juju show-machine 0
    juju show-machine 1 2 3


## Details

Shows a specified machine on a model.  The default format is `yaml`;
other formats can be specified with the `--format` option.
Available formats are `yaml`, `tabular`, and `json`.