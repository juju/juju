> See also: [ssh](#ssh), [debug-hooks](#debug-hooks)

## Summary
Launch a tmux session to debug hooks and/or actions.

## Usage
```juju debug-code [options] <unit name> [hook or action names]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--at` | all | will set the JUJU_DEBUG_AT environment variable to this value, which will then be interpreted by the charm for where you want to stop, defaults to 'all' |
| `--container` |  | the container name of the target pod |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `--no-host-key-checks` | false | Skip host key checking (INSECURE) |
| `--proxy` | false | Proxy through the API server |
| `--pty` | &lt;auto&gt; | Enable pseudo-tty allocation |
| `--remote` | false | Target on the workload or operator pod (k8s-only) |

## Examples

Debug all hooks and actions of unit '0':

    juju debug-code mysql/0

Debug all hooks and actions of the leader:

    juju debug-code mysql/leader

Debug the 'config-changed' hook of unit '1':

    juju debug-code mysql/1 config-changed

Debug the 'pull-site' action and 'update-status' hook:

    juju debug-code hello-kubecon/0 pull-site update-status

Debug the 'leader-elected' hook and set 'JUJU_DEBUG_AT' variable to 'hook':

    juju debug-code --at=hook mysql/0 leader-elected


## Details

The command launches a tmux session that will intercept matching hooks and/or
actions.

Initially, the tmux session will take you to '/var/lib/juju' or '/home/ubuntu'.
As soon as a matching hook or action is fired, the hook or action is executed
and the JUJU_DEBUG_AT variable is set. Charms implementing support for this
should set debug breakpoints based on the environment variable. Charms written
with the Ops library automatically provide support for this.

Valid unit identifiers are:
  a standard unit ID, such as mysql/0 or;
  leader syntax of the form &lt;application&gt;/leader, such as mysql/leader.

If no hook or action is specified, all hooks and actions will be intercepted.

See the "juju help ssh" for information about SSH related options
accepted by the debug-code command.



