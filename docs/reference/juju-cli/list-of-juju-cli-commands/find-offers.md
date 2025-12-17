(command-juju-find-offers)=
# `juju find-offers`
> See also: [show-offer](#show-offer)

## Summary
Finds offered application endpoints.

## Usage
```juju find-offers [options] ```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Specifies whether to skip web browser for authentication. |
| `--format` | tabular | Specify output format (json&#x7c;tabular&#x7c;yaml) |
| `--interface` |  | Returns results matching the interface name. |
| `-m`, `--model` |  | Specifies the model to operate in. Accepts `[<controller name>:]<model name>|<model UUID>`. |
| `-o`, `--output` |  | Specify an output file |
| `--offer` |  | Returns results matching the offer name. |
| `--url` |  | Returns results matching the offer URL. |

## Examples

    juju find-offers
    juju find-offers mycontroller:
    juju find-offers fred/prod
    juju find-offers --interface mysql
    juju find-offers --url fred/prod.db2
    juju find-offers --offer db2



## Details

Finds which offered application endpoints are available to the current user.

This command is aimed for a user who wants to discover what endpoints are available to them.