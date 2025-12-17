(command-juju-resume-relation)=
# `juju resume-relation`
> See also: [integrate](#integrate), [offers](#offers), [remove-relation](#remove-relation), [suspend-relation](#suspend-relation)

## Summary
Resumes a suspended relation to an application offer.

## Usage
```juju resume-relation [options] <relation-id>[,<relation-id>]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Specifies whether to skip web browser for authentication. |
| `-m`, `--model` |  | Specifies the model to operate in. Accepts `[<controller name>:]<model name>|<model UUID>`. |

## Examples

    juju resume-relation 123
    juju resume-relation 123 456


## Details

Resumes a relation between an application in another model and an offer in this model.

The `relation-joined` and `relation-changed` hooks will be run for the relation, and the relation
status will be set to joined. The relation is specified using its ID.