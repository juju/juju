(command-juju-show-operation)=
# `juju show-operation`
> See also: [run](#run), [operations](#operations), [show-task](#show-task)

## Summary
Show results of an operation.

## Usage
```juju show-operation [options] <operation-id>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--format` | yaml | Specify output format (json&#x7c;yaml) |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `-o`, `--output` |  | Specify an output file |
| `--utc` | false | Show times in UTC |
| `--wait` | -1s | Wait for results |
| `--watch` | false | Wait indefinitely for results |

## Examples

    juju show-operation 1
    juju show-operation 1 --wait=2m
    juju show-operation 1 --watch


## Details

Show the results returned by an operation with the given ID.
To block until the result is known completed or failed, use
the `--wait` option with a duration, as in `--wait 5s` or `--wait 1h`.
Use `--watch` to wait indefinitely.

The default behavior without `--wait` or `--watch` is to immediately check and return;
if the results are `pending`, then only the available information will be
displayed.  This is also the behavior when any negative time is given.