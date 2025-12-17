(command-juju-debug-code)=
# `juju debug-code`
> See also: [ssh](#ssh), [debug-hooks](#debug-hooks)

## Summary
Launches a tmux session to debug hooks and/or actions.

## Usage
```juju debug-code [options] <unit name> [hook or action names]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--at` | all | Specifies the value that the `JUJU_DEBUG_AT` environment variable will be set to. This variable tells the charm where you want to stop. |
| `--container` |  | Specifies the container name of the target pod. |
| `-m`, `--model` |  | Specifies the model to operate in. Accepts `[<controller name>:]<model name>|<model UUID>`. |
| `--no-host-key-checks` | false | (INSECURE) Specifies whether to skip host key checking. |
| `--proxy` | false | Specifies whether to proxy through the API server. |
| `--pty` | &lt;auto&gt; | Enables pseudo-tty allocation. |
| `--remote` | false | (KUBERNETES ONLY) Targets the workload or operator pod. |

## Examples

Debug all hooks and actions of unit `mysql/0`:

    juju debug-code mysql/0

Debug all hooks and actions of the `mysql/leader` unit:

    juju debug-code mysql/leader

Debug the `config-changed` hook of unit '1':

    juju debug-code mysql/1 config-changed

Debug the `pull-site action` and `update-status` hook of the `hello-kubecon` charm:

    juju debug-code hello-kubecon/0 pull-site update-status

Debug the `leader-elected` hook and set `JUJU_DEBUG_AT` variable to `hook`:

    juju debug-code --at=hook mysql/0 leader-elected


## Details

Launches a `tmux` session that will intercept matching hooks and/or
actions.

Initially, the `tmux` session will take you to `/var/lib/juju` or `/home/ubuntu`.
As soon as a matching hook or action is fired, the hook or action is executed
and the `JUJU_DEBUG_AT` variable is set. Charms implementing support for this
should set debug breakpoints based on the environment variable. Charms written
with the Ops library automatically provide support for this.

Valid unit identifiers are:
- a standard unit ID, such as `mysql/0` or;
- leader syntax of the form `<application>/leader`, such as `mysql/leader`.

If no hook or action is specified, all hooks and actions will be intercepted.

See `juju help ssh` for information about SSH related options
accepted by the `debug-code` command.