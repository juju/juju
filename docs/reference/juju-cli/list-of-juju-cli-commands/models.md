(command-juju-models)=
# `juju models`
> See also: [add-model](#add-model)

**Aliases:** list-models

## Summary
Lists models a user can access on a controller.

## Usage
```juju models [options] ```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Disables web browser for authentication. |
| `--all` | false | Lists all models, regardless of user accessibility (administrative users only). |
| `-c`, `--controller` |  | Performs the operation in the specified controller. |
| `--exact-time` | false | Uses full timestamps. |
| `--format` | tabular | Specify output format (json&#x7c;tabular&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |
| `--user` |  | Specifies the user to list models for (administrative users only). |
| `--uuid` | false | Displays UUID for models. |

## Examples

    juju models
    juju models --user bob


## Details

The models listed here are either models created by the user, or
models which have been shared with the user. Default values for user and
controller are, respectively, the current user and the current controller.
The active model is denoted by an asterisk.