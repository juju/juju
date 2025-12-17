(command-juju-rename-space)=
# `juju rename-space`
> See also: [add-space](#add-space), [spaces](#spaces), [reload-spaces](#reload-spaces), [remove-space](#remove-space), [show-space](#show-space)

## Summary
Renames a network space.

## Usage
```juju rename-space [options] <old-name> <new-name>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Specifies whether to skip web browser for authentication. |
| `-m`, `--model` |  | Specifies the model to operate in. Accepts `[<controller name>:]<model name>|<model UUID>`. |
| `--rename` |  | Specifies the new name for the network space. |

## Examples

Rename a space from `db` to `fe`:

	juju rename-space db fe


## Details
Renames an existing space from `old-name` to `new-name`. Does not change the
associated subnets and `new-name` must not match another existing space.