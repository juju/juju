(command-juju-secrets)=
# `juju secrets`
> See also: [add-secret](#add-secret), [remove-secret](#remove-secret), [show-secret](#show-secret), [update-secret](#update-secret)

**Aliases:** list-secrets

## Summary
Lists secrets available in the model.

## Usage
```juju secrets [options] ```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--format` | tabular | Specify output format (json&#x7c;tabular&#x7c;yaml) |
| `-m`, `--model` |  | Specifies the model to operate in. Accepts `[<controller name>:]<model name>|<model UUID>`. |
| `-o`, `--output` |  | Specify an output file |
| `--owner` |  | Includes secrets for the specified owner. |

## Examples

    juju secrets
    juju secrets --format yaml


## Details

Displays the secrets available for charms to use if granted access.