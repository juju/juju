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
| `--reveal` | false | Include sensitive backend settings in YAML or JSON (controller superusers only) |

## Examples

    juju secret-backends
    juju secret-backends --format yaml


## Details

Lists registrations on the specified controller. The table shows Name, Type,
Secrets and Message, ordered by type and name. Use YAML or JSON for configuration
and full status messages. Only these formats accept `--reveal`; the table ignores
it with a notice. An empty result prints a notice and no formatted collection.
To inspect one name, use show-secret-backend instead.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/secret-backend/