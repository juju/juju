(command-juju-suspend-relation)=
# `juju suspend-relation`
> See also: [integrate](#integrate), [offers](#offers), [remove-relation](#remove-relation), [resume-relation](#resume-relation)

## Summary
Suspends a relation to an application offer.

## Usage
```juju suspend-relation [options] <relation-id>[ <relation-id>...]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Specifies whether to skip web browser for authentication. |
| `-m`, `--model` |  | Specifies the model to operate in. Accepts `[<controller name>:]<model name>|<model UUID>`. |
| `--message` |  | Specifies the reason for suspension. |

## Examples

    juju suspend-relation 123
    juju suspend-relation 123 --message "reason for suspending"
    juju suspend-relation 123 456 --message "reason for suspending"


## Details

Suspends a relation between an application in another model and an offer in this model.
The `relation-departed` and `relation-broken` hooks will be run for the relation, and the relation
status will be set to suspended. The relation is specified using its ID.