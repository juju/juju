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
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |

## Examples

    juju resume-relation 123
    juju resume-relation 123 456


## Details

A relation between an application in another model and an offer in this model will be resumed.
The `relation-joined` and `relation-changed` hooks will be run for the relation, and the relation
status will be set to joined. The relation is specified using its ID.