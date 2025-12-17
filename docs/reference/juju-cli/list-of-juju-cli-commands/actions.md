(command-juju-actions)=
# `juju actions`
> See also: [run](#run), [show-action](#show-action)

**Aliases:** list-actions

## Summary
Lists actions defined for an application.

## Usage
```juju actions [options] <application>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Specifies whether to skip web browser for authentication. |
| `--format` | default | Specify output format (default&#x7c;json&#x7c;tabular&#x7c;yaml) |
| `-m`, `--model` |  | Specifies the model to operate in. Accepts `[<controller name>:]<model name>|<model UUID>`. |
| `-o`, `--output` |  | Specify an output file |
| `--schema` | false | Specifies whether to display the full action schema. |

## Examples

    juju actions postgresql
    juju actions postgresql --format yaml
    juju actions postgresql --schema


## Details

Lists the actions available to run on the target application, with a short
description.