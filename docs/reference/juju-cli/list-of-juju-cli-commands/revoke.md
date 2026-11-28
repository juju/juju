(command-juju-revoke)=
# `juju revoke`
> See also: [grant](#command-juju-grant)

## Summary
Revokes access from a Juju user for a model, controller, or application offer.

## Usage
```text
juju revoke [options] <user name> <permission> [<model name> ... | <offer url> ...]
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-c`, `--controller` |  | Controller to operate in |

## Examples

Remove Joe's direct grant on mymodel after checking that Joe holds model access directly:

    juju revoke joe read mymodel

Reduce Sam's direct grant to read on model1 and model2, where Sam holds at least write:

    juju revoke sam write model1 model2

Remove Joe's direct grant on fred/prod.hosted-mysql after checking Joe's grant and relations:

    juju revoke joe read fred/prod.hosted-mysql

Reduce Sam's direct grant to read on fred/prod.hosted-mysql and mary/test.hosted-mysql, where Sam holds at least consume. Check Sam's relations to both offers first:

    juju revoke sam consume fred/prod.hosted-mysql mary/test.hosted-mysql


## Details
By default, the controller is the current controller.

Omit the target to revoke controller access. To revoke model or offer access,
list either model names or offer URLs. Use juju revoke-cloud for clouds.

Check the user's direct grant on each target and choose the level they hold
or a lower one. Revoking the lowest level removes the direct grant. Before
lowering offer access below consume, have an admin of the offering model or
a superuser on its controller check the user's direct grant and connections.

Valid access levels for models are:
    read
    write
    admin

Valid access levels for controllers are:
    login
    superuser

Valid access levels for application offers are:
    read
    consume
    admin

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/user/