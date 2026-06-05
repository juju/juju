(command-juju-constraints)=
# `juju constraints`
> See also: [set-constraints](#set-constraints), [model-constraints](#model-constraints), [set-model-constraints](#set-model-constraints)

## Summary
Displays machine constraints for an application.

## Usage
```juju constraints [options] <application>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--format` | constraints | Specify output format (constraints&#x7c;json&#x7c;yaml) |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `-o`, `--output` |  | Specify an output file |

## Examples

    juju constraints mysql
    juju constraints -m mymodel apache2


## Details

Shows machine constraints that have been set for an application with
`juju set-constraints`.

By default, the model is the current model.

Where model and application constraints overlap, the application constraints take precedence.