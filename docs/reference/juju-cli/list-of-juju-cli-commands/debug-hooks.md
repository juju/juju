(command-juju-debug-hooks)=
# `juju debug-hooks`
> See also: [ssh](#command-juju-ssh), [debug-code](#command-juju-debug-code)

**Aliases:** debug-hook

## Summary
Launch a tmux session to debug hooks and/or actions.

## Usage
```text
juju debug-hooks [options] <unit name> [hook or action names]
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--container` |  | the container name of the target pod |
| `--direct` | false | Use the legacy machine SSH or Kubernetes API route instead of the SSH jump server |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `--no-host-key-checks` | false | Skip host key checking (INSECURE) |
| `--print-keys` | false | Print SHA256 fingerprints of the controller and virtual host keys |
| `--proxy` | &lt;auto&gt; | Proxy through the controller machine's OpenSSH server (requires `--direct`; defaults to the model's `proxy-ssh` setting) |
| `--pty` | &lt;auto&gt; | Enable pseudo-tty allocation |
| `--show-command` | false | Print the OpenSSH command for the jump route instead of running it |
| `--ssh-key` |  | Private key to offer to the controller's SSH jump server |

## Examples

Debug all hooks and actions of unit `mysql/0`:

    juju debug-hooks mysql/0

Debug all hooks and actions of the current `mysql` leader:

    juju debug-hooks mysql/leader

Debug the `config-changed` hook of unit `mysql/1`:

    juju debug-hooks mysql/1 config-changed

Debug the `pull-site` action and the `update-status` hook of unit `hello-kubecon/0`:

    juju debug-hooks hello-kubecon/0 pull-site update-status


## Details

The command launches a `tmux` session that will intercept matching hooks and/or
actions.

Initially, the `tmux` session will take you to `/var/lib/juju` or `/home/ubuntu`.
As soon as a matching hook or action is fired, the `tmux` session will
automatically navigate you to `/var/lib/juju/agents/<unit-id>/charm`with a
properly configured environment. Unlike the `juju debug-code` command,
the fired hooks and/or actions are not executed directly; instead, the user
needs to manually run the dispatch script inside the charm's directory.

Valid unit identifiers are:
- a standard unit ID, such as `mysql/0` or;
- leader syntax of the form `<application>/leader`, such as `mysql/leader`.

If no hook or action is specified, all hooks and actions will be intercepted.

See `juju help ssh` for information about SSH related options
accepted by the `debug-hooks` command.