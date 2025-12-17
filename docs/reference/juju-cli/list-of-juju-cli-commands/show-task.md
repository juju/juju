(command-juju-show-task)=
# `juju show-task`
> See also: [cancel-task](#cancel-task), [run](#run), [operations](#operations), [show-operation](#show-operation)

## Summary
Shows results of a task by ID.

## Usage
```juju show-task [options] <task ID>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Specifies whether to skip web browser for authentication. |
| `--format` | plain | Specify output format (json&#x7c;plain&#x7c;yaml) |
| `-m`, `--model` |  | Specifies the model to operate in. Accepts `[<controller name>:]<model name>|<model UUID>`. |
| `-o`, `--output` |  | Specify an output file |
| `--utc` | false | Specifies whether to show times in UTC. |
| `--wait` | -1s | Specifies the maximum wait time for a task to complete. |
| `--watch` | false | Specifies whether to wait indefinitely for results. |

## Examples

    juju show-task 1
    juju show-task 1 --wait=2m
    juju show-task 1 --watch


## Details

Shows the results returned by a task with the given ID.

To block until the result is known completed or failed, use
the `--wait` option with a duration, as in `--wait 5s` or `--wait 1h`.
Use `--watch` to wait indefinitely.

The default behavior without `--wait` or `--watch` is to immediately check and return;
if the results are `pending`, then only the available information will be
displayed.  This is also the behavior when any negative time is given.

Note: If Juju has been upgraded from 2.6 and there are old action UUIDs still in use,
and you want to specify just the UUID prefix to match on, you will need to include up
to at least the first `-` to disambiguate from a newer numeric ID.