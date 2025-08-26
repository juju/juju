(command-juju-revoke-cloud)=
# `juju revoke-cloud`
> See also: [grant-cloud](#grant-cloud)

## Summary
Revokes access from a Juju user for a cloud.

## Usage
```juju revoke-cloud [options] <user name> <permission> <cloud name> ...```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-c`, `--controller` |  | Controller to operate in |

## Examples

Revoke `add-model` (and 'admin') access from user `joe` for cloud `fluffy`:

    juju revoke-cloud joe add-model fluffy

Revoke `admin` access from user `sam` for clouds `fluffy` and `rainy`:

    juju revoke-cloud sam admin fluffy rainy



## Details

Revoking admin access, from a user who has that permission, will leave
that user with `add-model` access. Revoking `add-model`access, however, also revokes
admin access.

Valid access levels are:
    admin
    add-model