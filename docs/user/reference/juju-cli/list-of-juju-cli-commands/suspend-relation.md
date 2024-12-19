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
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `--message` |  | reason for suspension |

## Examples

    juju suspend-relation 123
    juju suspend-relation 123 --message "reason for suspending"
    juju suspend-relation 123 456 --message "reason for suspending"


## Details

A relation between an application in another model and an offer in this model will be suspended. 
The relation-departed and relation-broken hooks will be run for the relation, and the relation
status will be set to suspended. The relation is specified using its id.