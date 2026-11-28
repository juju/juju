(command-juju-revoke-cloud)=
# `juju revoke-cloud`
> See also: [grant-cloud](#command-juju-grant-cloud)

## Summary
Revokes access from a Juju user for a cloud.

## Usage
```text
juju revoke-cloud [options] <user name> <permission> <cloud name> ...
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-c`, `--controller` |  | Controller to operate in |

## Examples

Remove Joe's direct grant on fluffy after checking that Joe holds cloud access directly:

    juju revoke-cloud joe add-model fluffy

Reduce Sam's direct grant to add-model on fluffy and rainy, where Sam holds admin on both:

    juju revoke-cloud sam admin fluffy rainy



## Details

Check the user's direct grant on each cloud and choose the level they hold
or a lower one. Revoking `admin` from a cloud administrator leaves `add-model`
access. Revoking `add-model`removes the direct grant. Ask a different
authorised administrator to change your own cloud grant.

Valid access levels are:
    admin
    add-model

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/cloud/