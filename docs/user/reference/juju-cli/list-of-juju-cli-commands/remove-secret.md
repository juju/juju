(command-juju-remove-secret)=
# `juju remove-secret`
## Summary
Remove a existing secret.

## Usage
```juju remove-secret [options] <ID>|<name>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `--revision` | 0 | remove the specified revision |

## Examples

    juju remove-secret my-secret
    juju remove-secret secret:9m4e2mr0ui3e8a215n4g
    juju remove-secret secret:9m4e2mr0ui3e8a215n4g --revision 4


## Details

Remove all the revisions of a secret with the specified URI or remove the provided revision only.