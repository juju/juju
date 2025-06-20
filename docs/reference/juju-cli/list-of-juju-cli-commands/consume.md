(command-juju-consume)=
# `juju consume`
> See also: [integrate](#integrate), [offer](#offer), [remove-saas](#remove-saas)

## Summary
Add a remote offer to the model.

## Usage
```juju consume [options] <remote offer path> [<local application name>]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |

## Examples

    juju consume othermodel.mysql
    juju consume prod/othermodel.mysql
    juju consume anothercontroller:prod/othermodel.mysql


## Details
Adds a remote offer to the model. Relations can be created later using "juju relate".

The path to the remote offer is formatted as follows:

    [<controller name>:][<model qualifier>/]<model name>.<application name>
        
If the controller name is omitted, Juju will use the currently active
controller. Similarly, if the model qualifier is omitted, Juju will use the user
that is currently logged in to the controller providing the offer.