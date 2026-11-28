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
| `--destroy-storage` | false | Request destruction of storage owned by the unit (machine models only) |
| `--dry-run` | false | Print what this command would remove without removing |
| `--force` | false | Completely remove an unit and all its dependencies |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `--no-prompt` | false | Do not ask for confirmation. Overrides `mode` model config setting |
| `--no-wait` | false | Rush through unit removal without waiting for each individual step to complete |
| `--num-units` | 0 | Number of units to remove (k8s models only) |

## Examples

Remove three named machine units:

    juju remove-unit wordpress/2 wordpress/3 wordpress/4

Remove a machine unit and request destruction of storage it owns:

    juju remove-unit wordpress/2 --destroy-storage

Force removal of a machine unit after normal recovery has failed:

    juju remove-unit wordpress/2 --force

Force removal of a machine unit after normal recovery has failed and skip the waits between removal steps:

    juju remove-unit wordpress/2 --force --no-wait

Reduce a Kubernetes application's unit count by two, without choosing units:

    juju remove-unit wordpress --num-units 2


## Details

Remove application units from the model.

The usage of this command differs depending on whether it is being used on a
Kubernetes or a machine model.

Removing all units leaves the application in the model. To remove the
application itself, use `juju remove-application`.

On Kubernetes, supply one application name and the number of units to
remove. Kubernetes selects the units.

In a machine model, pass the names of the units to remove as separate
arguments. For example, `wordpress/3` identifies one unit of wordpress.

Removing the last unit on a machine can also retire the machine. Check where
its data is stored before confirming removal.

A failed hook can block normal removal. If recovery has failed, the
`--force` option can remove the unit and its machine before they shut
down cleanly. With forced removal, `--no-wait` skips the wait between steps.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/unit/