(command-juju-secret-backends)=
# `juju secret-backends`
> See also: [add-secret-backend](#add-secret-backend), [remove-secret-backend](#remove-secret-backend), [show-secret-backend](#show-secret-backend), [update-secret-backend](#update-secret-backend)

## Summary
Lists secret backends available in the controller.

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-c`, `--controller` |  | Controller to operate in |
| `--format` | tabular | Specify output format (json&#x7c;tabular&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |
| `--reveal` | false | Include sensitive backend config content |

## Examples

    juju secret-backends
    juju secret-backends --format yaml


## Details

Displays the secret backends available for storing secret content.