(command-juju-offer)=
# `juju offer`
> See also: [consume](#command-juju-consume), [integrate](#command-juju-integrate), [remove-saas](#command-juju-remove-saas)

## Summary
Offer application endpoints for use in other models.

## Usage
```text
juju offer [options] [[<model-qualifier>/]<model-name>.]<application-name>:<endpoint-name>[,<endpoint-name>...] [<offer-name>]
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-c`, `--controller` |  | Controller to operate in |

## Examples

    juju offer mysql:db
    juju offer staging/mymodel.mysql:db hosted-mysql
    juju offer db2:db,log hosted-db2


## Details

Give one or more endpoint names from the same application, separated by commas.
Without a model prefix, Juju uses the current model on the selected controller.
The offer name defaults to the application name. The command prints the offer URL.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/offer/