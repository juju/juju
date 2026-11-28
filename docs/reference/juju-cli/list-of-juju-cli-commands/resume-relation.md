(command-juju-resume-relation)=
# `juju resume-relation`
> See also: [integrate](#command-juju-integrate), [offers](#command-juju-offers), [remove-relation](#command-juju-remove-relation), [suspend-relation](#command-juju-suspend-relation)

## Summary
Resumes a suspended relation to an application offer.

## Usage
```text
juju resume-relation [options] <relation-id>[ <relation-id>...]
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |

## Examples

Resume relation 123 in the payments model:

    juju resume-relation -m payments 123

Resume two suspended relations in the payments model using their space-separated IDs:

    juju resume-relation -m payments 123 456


## Details

A relation between an application in another model and an offer in this model will be resumed.
The `relation-joined` and `relation-changed` hooks will be run for the relation, and the relation
status will be set to joined. The relation is specified using its ID.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/relation/