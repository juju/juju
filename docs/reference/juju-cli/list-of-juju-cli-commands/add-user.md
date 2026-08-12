(command-juju-add-user)=
# `juju add-user`
> See also: [register](#command-juju-register), [grant](#command-juju-grant), [users](#command-juju-users), [show-user](#command-juju-show-user), [disable-user](#command-juju-disable-user), [enable-user](#command-juju-enable-user), [change-user-password](#command-juju-change-user-password), [remove-user](#command-juju-remove-user)

## Summary
Adds a Juju user to a controller.

## Usage
```text
juju add-user [options] <user name> [<display name>]
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-c`, `--controller` |  | Controller to operate in |

## Examples

    juju add-user bob
    juju add-user --controller mycontroller bob


## Details

The user's details are stored within the controller and will be removed when
the controller is destroyed.

A user unique registration string will be printed. This registration string 
must be used by the newly added user as supplied to complete the registration
process.

Some machine providers will require the user to be in possession of certain
credentials in order to create a model.