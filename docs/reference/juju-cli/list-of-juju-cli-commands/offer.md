(command-juju-offer)=
# `juju offer`
> See also: [consume](#consume), [integrate](#integrate), [remove-saas](#remove-saas)

## Summary
Offer application endpoints for use in other models.

## Usage
```juju offer [options] [model-name.]<application-name>:<endpoint-name>[,...] [offer-name]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-c`, `--controller` |  | Controller to operate in |

## Examples

    juju offer mysql:db
    juju offer mymodel.mysql:db
    juju offer db2:db hosted-db2
    juju offer db2:db,log hosted-db2


## Details

Deployed application endpoints are offered for use by consumers.
By default, the offer is named after the application, unless
an offer name is explicitly specified.