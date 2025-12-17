(command-juju-destroy-controller)=
# `juju destroy-controller`
> See also: [kill-controller](#kill-controller), [unregister](#unregister)

## Summary
Destroys a controller.

## Usage
```juju destroy-controller [options] <controller name>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Specifies whether to skip web browser for authentication. |
| `--destroy-all-models` | false | Destroys all models in the controller. |
| `--destroy-storage` | false | Destroys all storage instances managed by the controller. |
| `--force` | false | Forcibly destroys the controller even if it is not empty. |
| `--model-timeout` | -1s | Specifies the timeout for each step of force model destruction. |
| `--no-prompt` | false | Specifies whether to skip confirmation prompt. |
| `--no-wait` | false | Rushes through model destruction without waiting for each individual step to complete. |
| `--release-storage` | false | Releases all storage instances from management of the controller, without destroying them. |

## Examples

Destroy the controller and all models. If there is
persistent storage remaining in any of the models, then
this will prompt you to choose to either destroy or release
the storage.

    juju destroy-controller --destroy-all-models mycontroller

Destroy the controller and all models, destroying
any remaining persistent storage.

    juju destroy-controller --destroy-all-models --destroy-storage

Destroy the controller and all models, releasing
any remaining persistent storage from Juju's control.

    juju destroy-controller --destroy-all-models --release-storage

Destroy the controller and all models, continuing
even if there are operational errors.

    juju destroy-controller --destroy-all-models --force
    juju destroy-controller --destroy-all-models --force --no-wait


## Details
All workload models running on the controller will first
need to be destroyed, either in advance, or by
specifying `--destroy-all-models`.

If there is persistent storage in any of the models managed by the
controller, then you must choose to either destroy or release the
storage, using `--destroy-storage` or `--release-storage` respectively.

Sometimes, the destruction of a model may fail as Juju encounters errors
that need to be dealt with before that model can be destroyed.
However, at times, there is a need to destroy a controller ignoring
such model errors. In these rare cases, use --force option but note
that --force will also remove all units of any hosted applications, their subordinates
and, potentially, machines without given them the opportunity to shutdown cleanly.

Model destruction is a multi-step process. Under normal circumstances, Juju will not
proceed to the next step until the current step has finished.
However, when using `--force`, users can also specify `--no-wait` to progress through steps
without delay waiting for each step to complete.

WARNING: Passing `--force` with `--model-timeout` will continue the final destruction without
consideration or respect for clean shutdown or resource cleanup. If `model-timeout`
elapses with `--force`, you may have resources left behind that will require
manual cleanup. If `--force --model-timeout 0` is passed, the models are brutally
removed with haste. It is recommended to use graceful destroy (without `--force`, `--no-wait` or
`--model-timeout`).