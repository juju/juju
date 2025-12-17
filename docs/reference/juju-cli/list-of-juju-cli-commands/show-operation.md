(command-juju-show-operation)=
# `juju show-operation`
> See also: [run](#run), [operations](#operations), [show-task](#show-task)

## Summary
Shows the results of an operation.

## Usage
```juju show-operation [options] <operation-id>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Specifies whether to skip web browser for authentication. |
| `--format` | yaml | Specify output format (json&#x7c;yaml) |
| `-m`, `--model` |  | Specifies the model to operate in. Accepts `[<controller name>:]<model name>|<model UUID>`. |
| `-o`, `--output` |  | Specify an output file |
| `--utc` | false | Specifies whether to show times in UTC. |
| `--wait` | -1s | Waits for results. |
| `--watch` | false | Specifies whether to wait indefinitely for results. |

## Examples

    juju show-operation 1
    juju show-operation 1 --wait=2m
    juju show-operation 1 --watch


## Details

Shows the results returned by an operation with the given ID.

To block until the result is known completed or failed, use
the `--wait` option with a duration, as in `--wait 5s` or `--wait 1h`.
Use `--watch` to wait indefinitely.

The default behavior without `--wait` or `--watch` is to immediately check and return;
if the results are `pending`, then only the available information will be
displayed.  This is also the behavior when any negative time is given.