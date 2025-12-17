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
| `-B`, `--no-browser-login` | false | Specifies whether to skip web browser for authentication. |
| `--color` | false | Specifies whether to force use of ANSI color codes. |
| `--format` | yaml | Specify output format (json&#x7c;tabular&#x7c;yaml) |
| `-m`, `--model` |  | Specifies the model to operate in. Accepts `[<controller name>:]<model name>|<model UUID>`. |
| `-o`, `--output` |  | Specify an output file |
| `--utc` | false | Specifies whether to display time as UTC in RFC3339 format. |

## Examples

    juju show-machine 0
    juju show-machine 1 2 3


## Details

Shows a specified machine on a model.

The default format is `yaml`;
other formats can be specified with the `--format` option.
Available formats are `yaml`, `tabular`, and `json`.