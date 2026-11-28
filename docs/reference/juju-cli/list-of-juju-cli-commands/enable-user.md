(command-juju-enable-user)=
# `juju enable-user`
> See also: [users](#command-juju-users), [disable-user](#command-juju-disable-user), [login](#command-juju-login)

## Summary
Re-enables a previously disabled Juju user.

## Usage
```text
juju enable-user [options] <user name>
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-c`, `--controller` |  | Controller to operate in |

## Examples

Restore Bob's sign-in on the current controller:

    juju enable-user bob


## Details
Re-enable an existing user as a controller superuser. Check the user's
retained grants before restoring sign-in. The command uses the current
controller by default.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/user/