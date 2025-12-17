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
| `-B`, `--no-browser-login` | false | Specifies whether to skip web browser for authentication. |
| `--all` | false | (ADMINS ONLY) Lists all models, regardless of user accessibility. |
| `-c`, `--controller` |  | Specifies the controller to operate in. |
| `--exact-time` | false | Uses full timestamps. |
| `--format` | tabular | Specify output format (json&#x7c;tabular&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |
| `--user` |  | (ADMINS ONLY) Specifies the user to list models for. |
| `--uuid` | false | Displays UUID for models. |

## Examples

    juju models
    juju models --user bob


## Details

The models listed here are either models you have created yourself, or
models which have been shared with you. Default values for user and
controller are, respectively, the current user and the current controller.
The active model is denoted by an asterisk.