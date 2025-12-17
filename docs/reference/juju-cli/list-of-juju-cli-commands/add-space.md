(command-juju-add-space)=
# `juju add-space`
> See also: [spaces](#spaces), [remove-space](#remove-space)

## Summary
Adds a new network space.

## Usage
```juju add-space [options] <name> [<CIDR1> <CIDR2> ...]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Specifies whether to skip web browser for authentication. |
| `-m`, `--model` |  | Specifies the model to operate in. Accepts `[<controller name>:]<model name>|<model UUID>`. |

## Examples


Add space `beta` with subnet `172.31.0.0/20`:

    juju add-space beta 172.31.0.0/20


## Details
Adds a new space with the given name and associates the given
(optional) list of existing subnet CIDRs with it.