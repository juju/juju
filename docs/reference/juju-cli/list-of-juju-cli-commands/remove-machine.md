(command-juju-remove-machine)=
# `juju remove-machine`
> See also: [add-machine](#add-machine)

## Summary
Removes one or more machines from a model.

## Usage
```juju remove-machine [options] <machine-id> ...```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Disables web browser for authentication. |
| `--dry-run` | false | Simulates the removal without making changes. |
| `--force` | false | Completely removes a machine and all its dependencies. |
| `--keep-instance` | false | Keeps the running cloud instance. |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `--no-prompt` | false | Skip confirmation. Overrides `mode` model config setting. |
| `--no-wait` | false | Rushes through machine removal without waiting for each individual step to complete. |

## Examples

    juju remove-machine 5
    juju remove-machine 6 --force
    juju remove-machine 6 --force --no-wait
    juju remove-machine 7 --keep-instance


## Details

Machines are specified by their numbers, which may be retrieved from the
output of `juju status`.

It is possible to remove a machine from Juju model without affecting
the corresponding cloud instance by using the `--keep-instance` option.

Machines responsible for the model cannot be removed.

Machines running units or containers can be removed using the `--force`
option; this will also remove those units and containers without giving
them an opportunity to shut down cleanly.

Machine removal is a multi-step process. Under normal circumstances, Juju will not
proceed to the next step until the current step has finished.
However, when using `--force`, users can also specify `--no-wait`
to progress through steps without delay waiting for each step to complete.