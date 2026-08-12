(command-juju-scale-application)=
# `juju scale-application`
> See also: [remove-application](#command-juju-remove-application), [add-unit](#command-juju-add-unit), [remove-unit](#command-juju-remove-unit)

## Summary
Set the desired number of k8s application units.

## Usage
```text
juju scale-application [options] <application> <scale>
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |

## Examples

    juju scale-application mariadb 2


## Details

Scale a Kubernetes application by specifying how many units there should be.
The new number of units can be greater or less than the current number, thus
allowing both scale out and scale in.