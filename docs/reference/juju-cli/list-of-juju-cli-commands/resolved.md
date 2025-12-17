(command-juju-resolved)=
# `juju resolved`
**Aliases:** resolve

## Summary
Marks unit errors resolved and re-executes failed hooks.

## Usage
```juju resolved [options] [<unit> ...]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Specifies whether to skip web browser for authentication. |
| `--all` | false | Specifies whether to mark all units in error as resolved. |
| `-m`, `--model` |  | Specifies the model to operate in. Accepts `[<controller name>:]<model name>|<model UUID>`. |
| `--no-retry` | false | Specifies whether to skip re-execution of failed hooks on the unit. |

## Examples


	juju resolved mysql/0

	juju resolved mysql/0 mysql/1

	juju resolved --all