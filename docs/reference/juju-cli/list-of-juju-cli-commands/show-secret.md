(command-juju-show-secret)=
# `juju show-secret`
> See also: [add-secret](#add-secret), [update-secret](#update-secret), [remove-secret](#remove-secret)

## Summary
Shows details for a specific secret.

## Usage
```juju show-secret [options] <ID>|<name>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--format` | yaml | Specify output format (json&#x7c;yaml) |
| `-m`, `--model` |  | Specifies the model to operate in. Accepts `[<controller name>:]<model name>|<model UUID>`. |
| `-o`, `--output` |  | Specify an output file |
| `-r`, `--revision` | 0 |  |
| `--reveal` | false | (YAML/JSON ONLY) Specifies whether to reveal secret values. |
| `--revisions` | false | Specifies whether to show the secret revisions metadata. |

## Examples

    juju show-secret my-secret
    juju show-secret 9m4e2mr0ui3e8a215n4g
    juju show-secret secret:9m4e2mr0ui3e8a215n4g --revision 2
    juju show-secret 9m4e2mr0ui3e8a215n4g --revision 2 --reveal
    juju show-secret 9m4e2mr0ui3e8a215n4g --revisions
    juju show-secret 9m4e2mr0ui3e8a215n4g --reveal


## Details

Displays the details of a specified secret.

For controller/model admins, the actual secret content is exposed
with the `--reveal` option in the `json` or `yaml` formats.

Use `--revision` to inspect a particular revision, else latest is used.
Use `--revisions` to see the metadata for each revision.