(command-juju-logout)=
# `juju logout`
> See also: [change-user-password](#command-juju-change-user-password), [login](#command-juju-login)

## Summary
Logs a Juju user out of a controller.

## Usage
```text
juju logout [options]
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-c`, `--controller` |  | Controller to operate in |
| `--force` | false | Force logout when a locally recorded password is detected |

## Examples

Clear the saved account for the current controller on this client:

    juju logout


## Details

This command clears the saved account for the selected controller on this
client. It uses the current controller by default. If the client still
stores a password for the account, logout reports `preventing account loss`.
Set a password you know before logging out. Use `--force` when you can
sign in again and intend to clear the saved credentials.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/user/