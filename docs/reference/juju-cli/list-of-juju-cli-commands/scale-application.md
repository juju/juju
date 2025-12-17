(command-juju-scale-application)=
# `juju scale-application`
> See also: [remove-application](#remove-application), [add-unit](#add-unit), [remove-unit](#remove-unit)

## Summary
Sets the desired number of Kubernetes application units.

## Usage
```juju scale-application [options] <application> <scale>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Specifies whether to skip web browser for authentication. |
| `-m`, `--model` |  | Specifies the model to operate in. Accepts `[<controller name>:]<model name>|<model UUID>`. |

## Examples

    juju scale-application mariadb 2


## Details

Scales a Kubernetes application by specifying how many units there should be.
The new number of units can be greater or less than the current number, thus
allowing both scale out and scale in.