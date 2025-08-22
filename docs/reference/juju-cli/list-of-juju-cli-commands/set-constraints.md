(command-juju-set-constraints)=
# `juju set-constraints`
> See also: [constraints](#constraints), [model-constraints](#model-constraints), [set-model-constraints](#set-model-constraints)

## Summary
Sets machine constraints for an application.

## Usage
```juju set-constraints [options] <application> <constraint>=<value> ...```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |

## Examples

    juju set-constraints mysql mem=8G cores=4
    juju set-constraints -m mymodel apache2 mem=8G arch=amd64


## Details
Sets constraints for an application, which are used for all new machines
provisioned for that application. They can be viewed with `juju constraints`.
By default, the model is the current model.
Application constraints are combined with model constraints, set with `juju 
set-model-constraints`, for commands (such as 'juju deploy') that
provision machines for applications. Where model and application constraints
overlap, the application constraints take precedence.
Constraints for a specific model can be viewed with `juju model-constraints`.
This command requires that the application to have at least one unit. To apply
constraints to
the first unit set them at the model level or pass them as an argument
when deploying.