(command-juju-set-model-constraints)=
# `juju set-model-constraints`
> See also: [models](#models), [model-constraints](#model-constraints), [constraints](#constraints), [set-constraints](#set-constraints)

## Summary
Sets machine constraints on a model.

## Usage
```juju set-model-constraints [options] <constraint>=<value> ...```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |

## Examples

    juju set-model-constraints cores=8 mem=16G
    juju set-model-constraints -m mymodel root-disk=64G


## Details

Sets constraints on the model that can be viewed with `juju model-constraints`.
By default, the model is the current model.
Model constraints are combined with constraints set for an application with
`juju set-constraints` for commands (such as `deploy`) that provision
machines/containers for applications. Where model and application constraints overlap, the
application constraints take precedence.
Constraints for a specific application can be viewed with `juju constraints`.