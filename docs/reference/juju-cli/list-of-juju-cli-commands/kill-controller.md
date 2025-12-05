(command-juju-kill-controller)=
# `juju kill-controller`
> See also: [destroy-controller](#destroy-controller), [unregister](#unregister)

## Summary
Forcibly terminates all machines and other associated resources for a Juju controller.

## Usage
```juju kill-controller [options] <controller name>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Disables web browser for authentication. |
| `--no-prompt` | false | Do not ask for confirmation. |
| `-t`, `--timeout` | 5m0s | Specifies the timeout before direct destruction. |

## Details

Forcibly destroys the specified controller.  If the API server is accessible,
this command will attempt to destroy the controller model and all models
and their resources.

If the API server is unreachable, the machines of the controller model will be
destroyed through the cloud provisioner.  If there are additional machines,
including machines within models, these machines will not be destroyed
and will never be reconnected to the Juju controller being destroyed.

The normal process of killing the controller will involve watching the
models as they are brought down in a controlled manner. If for some reason the
models do not stop cleanly, there is a default five minute timeout. If no change
in the model state occurs for the duration of this timeout, the command will
stop watching and destroy the models directly through the cloud provider.