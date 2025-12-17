(command-juju-remove-secret)=
# `juju remove-secret`
## Summary
Removes an existing secret.

## Usage
```juju remove-secret [options] <ID>|<name>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-m`, `--model` |  | Specifies the model to operate in. Accepts `[<controller name>:]<model name>|<model UUID>`. |
| `--revision` | 0 | Removes the specified revision. |

## Examples

    juju remove-secret my-secret
    juju remove-secret secret:9m4e2mr0ui3e8a215n4g
    juju remove-secret secret:9m4e2mr0ui3e8a215n4g --revision 4


## Details

Removes all the revisions of a secret with the specified URI or removes the provided revision only.