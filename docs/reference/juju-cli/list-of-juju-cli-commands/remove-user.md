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

Remove Bob from the current controller and verify the name at the prompt:

    juju remove-user bob

Remove Bob from the operations controller after verifying the account:

    juju remove-user -c operations bob


## Details
Remove a user permanently as a controller superuser. The command uses the
current controller by default. Before removing the user, give another
enabled user explicit model admin access on each model that would lose
its last enabled administrator. Arrange any credential handover first.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/user/