(command-juju-cancel-task)=
# `juju cancel-task`
> See also: [show-task](#show-task)

## Summary
Cancels pending or running tasks.

## Usage
```juju cancel-task [options] (<task-id>|<task-id-prefix>) [...]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Specifies whether to skip web browser for authentication. |
| `--format` | yaml | Specify output format (json&#x7c;yaml) |
| `-m`, `--model` |  | Specifies the model to operate in. Accepts `[<controller name>:]<model name>|<model UUID>`. |
| `-o`, `--output` |  | Specify an output file |

## Examples

To cancel a task by ID:

    juju cancel-task 1

To cancel multiple tasks by ID:

    juju cancel-task 1 2 3


## Details

Cancels pending or running tasks matching given IDs or partial ID prefixes.