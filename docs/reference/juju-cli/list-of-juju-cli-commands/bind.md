(command-juju-bind)=
# `juju bind`
> See also: [spaces](#spaces), [show-space](#show-space), [show-application](#show-application)

## Summary
Changes bindings for a deployed application.

## Usage
```juju bind [options] <application> [<default-space>] [<endpoint-name>=<space> ...]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Specifies whether to skip web browser for authentication. |
| `--force` | false | Specifies whether to allow endpoints to be bound to spaces that might not be available to all existing units. |
| `-m`, `--model` |  | Specifies the model to operate in. Accepts `[<controller name>:]<model name>|<model UUID>`. |

## Examples

To update the default binding for the application and automatically update all
existing endpoint bindings that were referencing the old default, you can use
the following syntax:

  juju bind foo new-default

To bind individual endpoints to a space you can use the following syntax:

  juju bind foo endpoint-1=space-1 endpoint-2=space-2

Finally, the above commands can be combined to update both the default space
and individual endpoints in one go:

  juju bind foo new-default endpoint-1=space-1



## Details

In order to be able to bind any endpoint to a space, all machines where the
application units are deployed to are required to be configured with an address
in that space. However, you can use the `--force` option to bypass this check.