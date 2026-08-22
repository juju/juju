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
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--dry-run` | false | Print what this command would be removed without removing |
| `--force` | false | Force removal despite errors accessing cloud resources |
| `--keep-instance` | false | Do not stop the running cloud instance |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `--no-prompt` | false | Do not ask for confirmation. Overrides `mode` model config setting |
| `--no-wait` | false | Rush through machine removal without waiting for each individual step to complete |

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

When `requires-prompts` is present in the model's `mode`
configuration, removing machines that host units or containers requires
confirmation. Use `--no-prompt` to skip this confirmation.

Machine removal is a multi-step process. Under normal circumstances, Juju will not
proceed to the next step until the current step has finished.
Use `--force` to continue removal despite errors accessing cloud
resources. This does not bypass any required confirmation. When using
`--force`, users can also specify `--no-wait` to progress
through steps without waiting for each step to complete.