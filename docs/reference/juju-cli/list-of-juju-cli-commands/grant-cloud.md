(command-juju-grant-cloud)=
# `juju grant-cloud`
> See also: [grant](#grant), [revoke-cloud](#revoke-cloud), [add-user](#add-user)

## Summary
Grants access level to a Juju user for a cloud.

## Usage
```juju grant-cloud [options] <user name> <permission> <cloud name> ...```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Disables web browser for authentication. |
| `-c`, `--controller` |  | Performs the operation in the specified controller. |

## Examples

Grant user `joe` `add-model` access to cloud `fluffy`:

    juju grant-cloud joe add-model fluffy


## Details
Valid access levels are:
    admin
    add-model