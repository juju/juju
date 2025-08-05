> See also: [unregister](#unregister), [revoke](#revoke), [show-user](#show-user), [users](#users), [disable-user](#disable-user), [enable-user](#enable-user), [change-user-password](#change-user-password)

## Summary
Deletes a Juju user from a controller.

## Usage
```juju remove-user [options] <user name>```

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




