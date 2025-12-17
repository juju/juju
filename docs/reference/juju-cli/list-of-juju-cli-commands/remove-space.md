(command-juju-remove-space)=
# `juju remove-space`
> See also: [add-space](#add-space), [spaces](#spaces), [reload-spaces](#reload-spaces), [rename-space](#rename-space), [show-space](#show-space)

## Summary
Removes a network space.

## Usage
```juju remove-space [options] <name>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Specifies whether to skip web browser for authentication. |
| `--force` | false | Removes the space even if there are existing bindings, constraints, or settings. |
| `-m`, `--model` |  | Specifies the model to operate in. Accepts `[<controller name>:]<model name>|<model UUID>`. |
| `-y`, `--yes` | false | Skips confirmation prompt. |

## Examples

Remove a space by name:

	juju remove-space db-space

Remove a space by name with force, without need for confirmation:

	juju remove-space db-space --force -y


## Details
Removes an existing Juju network space with the given name. Any subnets
associated with the space will be transferred to the default space.
The command will fail if existing constraints, bindings or controller settings
are bound to the given space.

If the `--force` option is specified, the space will be deleted even
if there are existing bindings, constraints or settings.