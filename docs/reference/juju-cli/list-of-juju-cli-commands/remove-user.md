(command-juju-remove-user)=
# `juju remove-user`
(command-juju-remove-user)=
# `juju remove-user`
> See also: [unregister](#command-juju-unregister), [revoke](#command-juju-revoke), [show-user](#command-juju-show-user), [users](#command-juju-users), [disable-user](#command-juju-disable-user), [enable-user](#command-juju-enable-user), [change-user-password](#command-juju-change-user-password)

## Summary
Deletes a Juju user from a controller.

## Usage
```text
juju remove-user [options] <user name>
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-c`, `--controller` |  | Controller to operate in |
| `-y`, `--yes` | false | Confirm deletion of the user |

## Examples

    juju remove-user bob
    juju remove-user bob --yes


## Details
This removes a user permanently.

By default, the controller is the current controller.