(command-juju-list-models)=
# `juju list-models`
> See also: [add-model](#add-model)

**Aliases:** list-models

## Summary
Lists models a user can access on a controller.

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--all` | false | Lists all models, regardless of user accessibility (administrative users only) |
| `-c`, `--controller` |  | Controller to operate in |
| `--exact-time` | false | Use full timestamps |
| `--format` | tabular | Specify output format (json&#x7c;tabular&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |
| `--user` |  | The user to list models for (administrative users only) |
| `--uuid` | false | Display UUID for models |

## Examples

    juju models
    juju models --user bob


## Details

The models listed here are either models you have created yourself, or
models which have been shared with you. Default values for user and
controller are, respectively, the current user and the current controller.
The active model is denoted by an asterisk.