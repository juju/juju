(command-juju-destroy-model)=
# `juju destroy-model`
> See also: [destroy-controller](#destroy-controller)

## Summary
Terminates all machines/containers and resources for a non-controller model.

## Usage
```juju destroy-model [options] [<controller name>:]<model name>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Specifies whether to skip web browser for authentication. |
| `--destroy-storage` | false | Specifies whether to destroy all storage instances in the model. |
| `--force` | false | Specifies whether to force model destruction, ignoring any errors. |
| `--no-prompt` | false | Specifies whether to skip confirmation prompt. |
| `--no-wait` | false | Specifies whether to rush through model destruction without waiting for each individual step to complete. |
| `--release-storage` | false | Specifies whether to release all storage instances from the model, and management of the controller, without destroying them. |
| `-t`, `--timeout` | -1s | Specifies the timeout for each step of force model destruction. |

## Examples

    juju destroy-model --no-prompt mymodel
    juju destroy-model --no-prompt mymodel --destroy-storage
    juju destroy-model --no-prompt mymodel --release-storage
    juju destroy-model --no-prompt mymodel --force
    juju destroy-model --no-prompt mymodel --force --no-wait


## Details

Destroys the specified model. This will result in the non-recoverable
removal of all the units operating in the model and any resources stored
there. Due to the irreversible nature of the command, it will prompt for
confirmation (unless overridden with the `-y` option) before taking any
action.

If there is persistent storage in any of the models managed by the
controller, then you must choose to either destroy or release the
storage, using `--destroy-storage` or `--release-storage` respectively.

Sometimes, the destruction of the model may fail as Juju encounters errors
and failures that need to be dealt with before a model can be destroyed.
However, at times, there is a need to destroy a model ignoring
all operational errors. In these rare cases, use the `--force` option but note
that `--force` will also remove all units of the application, its subordinates
and, potentially, machines without given them the opportunity to shutdown cleanly.

Model destruction is a multi-step process. Under normal circumstances, Juju will not
proceed to the next step until the current step has finished.
However, when using `--force`, users can also specify `--no-wait` to progress through steps
without delay waiting for each step to complete.

WARNING: Passing `--force` with `--timeout` will continue the final destruction without
consideration or respect for clean shutdown or resource cleanup. If timeout
elapses with `--force`, you may have resources left behind that will require
manual cleanup. If `--force --timeout 0` is passed, the model is brutally
removed with haste. It is recommended to use graceful destroy (without `--force` or `--no-wait`).