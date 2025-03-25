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
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--all` | false | Marks all units in error as resolved |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `--no-retry` | false | Do not re-execute failed hooks on the unit |

## Examples


	juju resolved mysql/0

	juju resolved mysql/0 mysql/1

	juju resolved --all