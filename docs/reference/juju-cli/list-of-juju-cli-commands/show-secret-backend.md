(command-juju-show-secret-backend)=
# `juju show-secret-backend`
> See also: [add-secret-backend](#command-juju-add-secret-backend), [secret-backends](#command-juju-secret-backends), [remove-secret-backend](#command-juju-remove-secret-backend), [update-secret-backend](#command-juju-update-secret-backend)

## Summary
Displays the specified secret backend.

## Usage
```text
juju show-secret-backend [options] <backend-name>
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-c`, `--controller` |  | Controller to operate in |
| `--format` | yaml | Specify output format (json&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |
| `--reveal` | false | Include sensitive backend config content |

## Examples

    juju show-secret-backend myvault
    juju secret-backends myvault --reveal


## Details

Displays the specified secret backend.