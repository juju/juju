(command-juju-grant-cloud)=
# `juju grant-cloud`
> See also: [grant](#command-juju-grant), [revoke-cloud](#command-juju-revoke-cloud), [add-user](#command-juju-add-user)

## Summary
Grants access level to a Juju user for a cloud.

## Usage
```text
juju grant-cloud [options] <user name> <permission> <cloud name> ...
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-c`, `--controller` |  | Controller to operate in |

## Examples

Grant user `joe` `add-model` access to cloud `fluffy`:

    juju grant-cloud joe add-model fluffy


## Details
Grant access on the named clouds registered with the selected controller.
Ask a different authorised administrator to change your own cloud grant.

Valid access levels are:
    admin
    add-model

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/cloud/