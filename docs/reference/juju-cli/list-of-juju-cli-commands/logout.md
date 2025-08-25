(command-juju-logout)=
# `juju logout`
> See also: [change-user-password](#change-user-password), [login](#login)

## Summary
Logs a Juju user out of a controller.

## Usage
```juju logout [options] ```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-c`, `--controller` |  | Controller to operate in |
| `--force` | false | Force logout when a locally recorded password is detected |

## Examples

    juju logout


## Details

If another client has logged in as the same user, they will remain logged
in. This command only affects the local client.

The command will fail if the user has not yet set a password
(`juju change-user-password`). This scenario is only possible after
`juju bootstrap`since `juju register` sets a password. The
failing behaviour can be overridden with the `--force` option.

If the same user is logged in with another client system, that user session
will not be affected by this command; it only affects the local client.

By default, the controller is the current controller.