(command-juju-switch)=
# `juju switch`
> See also: [controllers](#controllers), [models](#models), [show-controller](#show-controller)

## Summary
Selects or identifies the current controller and model.

## Usage
```juju switch [options] [<controller>|<model>|<controller>:|:<model>|<controller>:<model>]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-c`, `--controller` |  | Controller to operate in |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt; |

## Examples

    juju switch
    juju switch mymodel
    juju switch mycontroller
    juju switch mycontroller:mymodel
    juju switch mycontroller:
    juju switch :mymodel
    juju switch -m mymodel
	juju switch -m mycontroller:mymodel
	juju switch -c mycontroller
    juju switch - # switch to previous controller:model
    juju switch -m - # switch to previous controller on its current model
    juju switch -c - # switch to previous model on the current controller


## Details
When used without an argument, the command shows the current controller
and its active model.

When a single argument without a colon is provided juju first looks for a
controller by that name and switches to it, and if it's not found it tries
to switch to a model within current controller. 

Colon allows to disambiguate model over controller:
- mycontroller: switches to default model in mycontroller, 
- :mymodel switches to mymodel in current controller 
- mycontroller:mymodel switches to mymodel on mycontroller.

The special arguments - (hyphen) instead of a model or a controller allows to return 
to previous model or controller. It can be used as main argument or as flag argument.

The `juju models` command can be used to determine the active model
(of any controller). An asterisk denotes it.