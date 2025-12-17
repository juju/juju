(command-juju-consume)=
# `juju consume`
> See also: [integrate](#integrate), [offer](#offer), [remove-saas](#remove-saas)

## Summary
Adds a remote offer to the model.

## Usage
```juju consume [options] <remote offer path> [<local application name>]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Specifies whether to skip web browser for authentication. |
| `-m`, `--model` |  | Specifies the model to operate in. Accepts `[<controller name>:]<model name>|<model UUID>`. |

## Examples

    juju consume othermodel.mysql
    juju consume owner/othermodel.mysql
    juju consume anothercontroller:owner/othermodel.mysql


## Details

Adds a remote offer to the model. Relations can be created later using `juju integrate`

The path to the remote offer is formatted as follows:

    [<controller name>:][<model owner>/]<model name>.<application name>

If the controller name is omitted, Juju will use the currently active
controller. Similarly, if the model owner is omitted, Juju will use the user
that is currently logged in to the controller providing the offer.