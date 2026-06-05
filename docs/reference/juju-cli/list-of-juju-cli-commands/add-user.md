(command-juju-add-user)=
# `juju add-user`
> See also: [register](#register), [grant](#grant), [users](#users), [show-user](#show-user), [disable-user](#disable-user), [enable-user](#enable-user), [change-user-password](#change-user-password), [remove-user](#remove-user)

## Summary
Adds a Juju user to a controller.

## Usage
```juju add-user [options] <user name> [<display name>]```

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