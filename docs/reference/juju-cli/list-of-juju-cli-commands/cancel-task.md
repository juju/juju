> See also: [show-task](#show-task)

## Summary
Cancel pending or running tasks.

## Usage
```juju cancel-task [options] (<task-id>|<task-id-prefix>) [...]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--format` | yaml | Specify output format (json&#x7c;yaml) |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `-o`, `--output` |  | Specify an output file |

## Examples

To cancel a task by ID:

    juju cancel-task 1

To cancel multiple tasks by ID:

    juju cancel-task 1 2 3


## Details

Cancel pending or running tasks matching given IDs or partial ID prefixes.


