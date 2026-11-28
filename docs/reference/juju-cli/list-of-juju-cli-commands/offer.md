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

Offer the `db` endpoint of `mysql` from the current model under the default offer name, `mysql`:

    juju offer mysql:db

Offer the `db` endpoint of `mysql` from model `staging/mymodel` under the name `hosted-mysql`:

    juju offer staging/mymodel.mysql:db hosted-mysql

Offer the `db` and `log` endpoints of `db2` together under the name `hosted-db2`:

    juju offer db2:db,log hosted-db2


## Details

Give one or more endpoint names from the same application, separated by commas.
Without a model prefix, Juju uses the current model on the selected controller.
The offer name defaults to the application name. The command prints the offer URL.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/offer/