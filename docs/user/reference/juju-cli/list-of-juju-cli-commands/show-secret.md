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
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `-o`, `--output` |  | Specify an output file |
| `-r`, `--revision` | 0 |  |
| `--reveal` | false | Reveal secret values, applicable to yaml or json formats only |
| `--revisions` | false | Show the secret revisions metadata |

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
with the '--reveal' option in json or yaml formats.

Use --revision to inspect a particular revision, else latest is used.
Use --revisions to see the metadata for each revision.