(command-juju-disable-user)=
# `juju disable-user`
> See also: [users](#command-juju-users), [enable-user](#command-juju-enable-user), [login](#command-juju-login)

## Summary
Disables a Juju user.

## Usage
```text
juju disable-user [options] <user name>
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-c`, `--controller` |  | Controller to operate in |

## Examples

Disable bob on the current controller:

    juju disable-user bob


## Details
Disabling a user prevents new sign-ins to this controller. Their access grants
are retained. Give another active user explicit admin access on each model
where the user is the last active model administrator.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/user/