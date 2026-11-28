(command-juju-change-user-password)=
# `juju change-user-password`
> See also: [add-user](#command-juju-add-user), [register](#command-juju-register)

## Summary
Changes the password for the current or specified Juju user.

## Usage
```text
juju change-user-password [options] [username]
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-c`, `--controller` |  | Controller to operate in |
| `--no-prompt` | false | Read a new password from a newline-terminated line on standard input when changing a password |
| `--reset` | false | Issue a new registration invitation for another named local user |

## Examples

Change your own local password on the current controller:

    juju change-user-password

Set Bob's password on the current controller as a controller superuser:

    juju change-user-password bob

Issue Bob a fresh registration invitation on the current controller as a controller superuser:

    juju change-user-password --reset bob

Change your own local password on the operations controller:

    juju change-user-password -c operations

Set Bob's local password on the operations controller as a controller superuser:

    juju change-user-password -c operations bob


## Details

Change the password for the local user signed in on the current controller.
Use `--controller` to select a different controller. A controller superuser
can provide a username to set another local user's password.

Use `--reset` as a controller superuser to invalidate another local user's
password and replace any outstanding invitation. With no username or with
your own username, the CLI prints guidance and makes no change. Reset reads
no password from standard input.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/user/