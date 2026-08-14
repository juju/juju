(command-juju-secret-backends)=
# `juju secret-backends`
> See also: [add-secret-backend](#command-juju-add-secret-backend), [remove-secret-backend](#command-juju-remove-secret-backend), [show-secret-backend](#command-juju-show-secret-backend), [update-secret-backend](#command-juju-update-secret-backend)

**Aliases:** list-secret-backends

## Summary
Lists secret backends available in the controller.

## Usage
```text
juju secret-backends [options]
```

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