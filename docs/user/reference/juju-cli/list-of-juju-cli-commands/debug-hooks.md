(command-juju-debug-hooks)=
# `juju debug-hooks`
> See also: [ssh](#ssh), [debug-code](#debug-code)

**Aliases:** debug-hook

## Summary
Launch a tmux session to debug hooks and/or actions.

## Usage
```juju debug-hooks [options] <unit name> [hook or action names]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--container` |  | the container name of the target pod |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `--no-host-key-checks` | false | Skip host key checking (INSECURE) |
| `--proxy` | false | Proxy through the API server |
| `--pty` | &lt;auto&gt; | Enable pseudo-tty allocation |
| `--remote` | false | Target on the workload or operator pod (k8s-only) |

## Examples

Debug all hooks and actions of unit '0':

    juju debug-hooks mysql/0

Debug all hooks and actions of the leader:

    juju debug-hooks mysql/leader

Debug the 'config-changed' hook of unit '1':

    juju debug-hooks mysql/1 config-changed

Debug the 'pull-site' action and 'update-status' hook of unit '0':

    juju debug-hooks hello-kubecon/0 pull-site update-status


## Details

The command launches a tmux session that will intercept matching hooks and/or
actions.

Initially, the tmux session will take you to '/var/lib/juju' or '/home/ubuntu'.
As soon as a matching hook or action is fired, the tmux session will
automatically navigate you to '/var/lib/juju/agents/&lt;unit-id&gt;/charm' with a
properly configured environment. Unlike the 'juju debug-code' command,
the fired hooks and/or actions are not executed directly; instead, the user
needs to manually run the dispatch script inside the charm's directory.

Valid unit identifiers are:
  a standard unit ID, such as mysql/0 or;
  leader syntax of the form &lt;application&gt;/leader, such as mysql/leader.

If no hook or action is specified, all hooks and actions will be intercepted.

See the "juju help ssh" for information about SSH related options
accepted by the debug-hooks command.