(command-juju-list-secrets)=
# `juju list-secrets`
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
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `-o`, `--output` |  | Specify an output file |
| `--owner` |  | Include secrets for the specified owner |

## Examples

    juju secrets
    juju secrets --format yaml


## Details

Displays the secrets