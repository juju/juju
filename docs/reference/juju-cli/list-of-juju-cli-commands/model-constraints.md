(command-juju-model-constraints)=
# `juju model-constraints`
> See also: [models](#models), [constraints](#constraints), [set-constraints](#set-constraints), [set-model-constraints](#set-model-constraints)

## Summary
Displays machine constraints for a model.

## Usage
```juju model-constraints [options] ```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--format` | constraints | Specify output format (constraints&#x7c;json&#x7c;yaml) |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `-o`, `--output` |  | Specify an output file |

## Examples

    juju model-constraints
    juju model-constraints -m mymodel


## Details
Shows constraints that have been set on the model with
`juju set-model-constraints.`
By default, the model is the current model.
Model constraints are combined with constraints set on an application
with `juju set-constraints` for commands (such as 'deploy') that provision
machines/containers for applications. Where model and application constraints overlap, the
application constraints take precedence.
Constraints for a specific application can be viewed with `juju constraints`.