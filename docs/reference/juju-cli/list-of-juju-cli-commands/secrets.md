(command-juju-secrets)=
# `juju secrets`
(command-juju-secrets)=
# `juju secrets`
> See also: [add-secret](#command-juju-add-secret), [remove-secret](#command-juju-remove-secret), [show-secret](#command-juju-show-secret), [update-secret](#command-juju-update-secret)

**Aliases:** list-secrets

## Summary
Lists secrets available in the model.

## Usage
```text
juju secrets [options]
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--format` | tabular | Specify output format (json&#x7c;tabular&#x7c;yaml) |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `-o`, `--output` |  | Specify an output file |
| `--owner` |  | Include secrets for the specified owner |
| `--revisions` | false | Show the secret revisions metadata |

## Examples

    juju secrets
    juju secrets --format yaml
    juju secrets --revisions --format yaml


## Details

Displays the secrets available for charms to use if granted access.