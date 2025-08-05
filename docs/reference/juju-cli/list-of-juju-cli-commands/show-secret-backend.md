> See also: [add-secret-backend](#add-secret-backend), [secret-backends](#secret-backends), [remove-secret-backend](#remove-secret-backend), [update-secret-backend](#update-secret-backend)

## Summary
Displays the specified secret backend.

## Usage
```juju show-secret-backend [options] <backend-name>```

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



