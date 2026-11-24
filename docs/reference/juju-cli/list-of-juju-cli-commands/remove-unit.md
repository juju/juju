(command-juju-remove-unit)=
# `juju remove-unit`
> See also: [remove-application](#command-juju-remove-application), [scale-application](#command-juju-scale-application)

## Summary
Remove application units from the model.

## Usage
```text
juju remove-unit [options] <unit> [...] | <application>
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--destroy-storage` | false | Destroy storage attached to the unit |
| `--dry-run` | false | Print what this command would remove without removing |
| `--force` | false | Completely remove an unit and all its dependencies |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `--no-prompt` | false | Do not ask for confirmation. Overrides `mode` model config setting |
| `--no-wait` | false | Rush through unit removal without waiting for each individual step to complete |
| `--num-units` | 0 | Number of units to remove (k8s models only) |

## Examples

Remove two units from an application on Kubernetes:

    juju remove-unit my-app --num-units 2

Remove named units on a machine model:

    juju remove-unit wordpress/2 wordpress/3 wordpress/4

Remove a unit on a machine model and destroy its attached storage:

    juju remove-unit wordpress/2 --destroy-storage

Force removal of a unit on a machine model:

    juju remove-unit wordpress/2 --force

Force removal on a machine model without waiting between removal steps:

    juju remove-unit wordpress/2 --force --no-wait


## Details

On machine models, specify one or more unit names, such as wordpress/3.

On Kubernetes models, specify one application and use --num-units to say how
many units to remove, not the number to keep. You cannot choose which units to
remove by name.

Removing all units does not remove the application itself. Use
`juju remove-application` to remove the application.

On a machine model, removing the last unit on a machine can also remove the
machine.

Sometimes, the removal of the unit may fail as Juju encounters errors
and failures that need to be dealt with before a unit can be removed.
For example, Juju will not remove a unit if there are hook failures.
However, at times, there is a need to remove a unit ignoring
all operational errors. In these rare cases, use --force option but note
that `--force` will remove a unit and, potentially, its machine without
given them the opportunity to shutdown cleanly.

Unit removal is a multi-step process. Under normal circumstances, Juju will not
proceed to the next step until the current step has finished.
However, when using `--force`, users can also specify `--no-wait`
to progress through steps without delay waiting for each step to complete.