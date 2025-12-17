(command-juju-remove-machine)=
# `juju remove-machine`
> See also: [add-machine](#add-machine)

## Summary
Removes one or more machines from a model.

## Usage
```juju remove-machine [options] <machine number> ...```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Specifies whether to skip web browser for authentication. |
| `--dry-run` | false | Specifies whether to merely simulate removal. |
| `--force` | false | Specifies whether to completely remove a machine and all its dependencies ignoring checks. |
| `--keep-instance` | false | Specifies whether to preserve the machine instance, but remove it from Juju. |
| `-m`, `--model` |  | Specifies the model to operate in. Accepts `[<controller name>:]<model name>|<model UUID>`. |
| `--no-prompt` | false | Specifies whether to skip confirmation prompt. Overrides `mode` model config setting. |
| `--no-wait` | false | Specifies whether to rush through machine removal without waiting for each individual step to complete. |

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