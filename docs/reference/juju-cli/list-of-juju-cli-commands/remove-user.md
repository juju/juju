(command-juju-remove-user)=
# `juju remove-user`
> See also: [unregister](#unregister), [revoke](#revoke), [show-user](#show-user), [users](#users), [disable-user](#disable-user), [enable-user](#enable-user), [change-user-password](#change-user-password)

## Summary
Deletes a Juju user from a controller.

## Usage
```juju remove-user [options] <user name>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Specifies whether to skip web browser for authentication. |
| `-c`, `--controller` |  | Specifies the controller to operate in. |
| `-y`, `--yes` | false | Specifies whether to confirm deletion of the user. |

## Examples

    juju remove-user bob
    juju remove-user bob --yes


## Details
Removes a user permanently.

By default, the controller is the current controller.