(command-juju-ssh)=
# `juju ssh`
> See also: [scp](#command-juju-scp)

## Summary
Initiates an SSH session or executes a command on a Juju machine or container.

## Usage
```text
juju ssh [options] <[user@]target> [openssh options] [command]
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

Connect to machine 0:

    juju ssh 0

Connect to machine 1 and run command 'uname -a':

    juju ssh 1 uname -a

Connect to the leader mysql unit:

    juju ssh mysql/leader

Connect to a specific mysql unit:

    juju ssh mysql/0

Print the controller and virtual host key fingerprints for a mysql unit:

    juju ssh --print-keys mysql/0

Connect directly to a machine-backed unit as a different remote user:

    juju ssh --direct jenkins@jenkins/0

Select a private key for the jump server:

    juju ssh --ssh-key ~/.ssh/my_private_key mysql/0 echo hello

When using the Juju snap, the u2f-devices interface must be
connected to allow access to FIDO/U2F security keys:

    sudo snap connect juju:u2f-devices

Select a FIDO/U2F security key (e.g. YubiKey) for the jump server:

    juju ssh --ssh-key ~/.ssh/id_ed25519_sk mysql/0

Connect to a k8s unit targeting the charm container (the default):

	juju ssh snappass/0
	juju ssh --container charm snappass/0

Connect to a k8s unit targeting the redis container:

	juju ssh --container redis snappass/0

Interact with the Pebble instance in the workload container via the charm container:

    juju ssh snappass/0 PEBBLE_SOCKET=/charm/containers/redis/pebble.socket /charm/bin/pebble plan

**For the Kubernetes controller model in direct mode only:**

Connect to controller pod 0 in the api-server or charm container:

    juju ssh --direct 0
    juju ssh --direct --container charm 0


## Details

By default, this command connects through the controller's SSH jump server.
Choose a target from `juju status`: a top-level machine number (e.g. `0`)
or a unit name (e.g. `mysql/0` or `mysql/leader`). In a Kubernetes model,
use a unit name, not a machine number.

For a machine or machine-backed unit, a command runs as `ubuntu`; omit the
command to open a shell. A `user@` prefix does not change this backend login.
For a Kubernetes unit, the command runs in the `charm` container by default.
Use `--container` to choose another container in the same pod. With no
command the client requests a Bash login shell. If Bash is not installed,
specify a shell the image provides, for example `juju ssh mysql/0 sh`.

When `juju ssh` is executed without a terminal attached, e.g., when piping the
output of another command into it, then the default behavior is to not allocate
a pseudo-terminal (pty) for the ssh session; otherwise a pty is allocated. This
behavior can be overridden by explicitly specifying the behavior with
`--pty=true` or `--pty=false`.

Select the private key for the jump server with `--ssh-key`. To pass options
to the local OpenSSH client, put them between the target and the remote
command. An `-i` option there does not select the jump identity.

For an older controller that does not support the jump route, specify
`--direct`; the client does not switch automatically. On machine models this
uses ordinary SSH, where `user@target` selects a remote account. On Kubernetes
it uses the Kubernetes API rather than the controller's SSH server.

On machine models, use `--direct --proxy` to route the connection through
the controller machine's OpenSSH server (port 22 by default). In direct mode,
if `--proxy` is omitted, Juju uses the model's `proxy-ssh` setting.
Use `--direct --proxy=false` to connect without this proxy.

On the jump route, the client checks both server host keys against keys
obtained from the Juju API by default. Disabling checks with `--no-host-key-checks`
risks connecting to an impostor. Use `--show-command` to print an OpenSSH
invocation rather than run the jump session. When you run it, OpenSSH uses its own host-key
configuration, not the client's temporary known-hosts file.

Use `--print-keys` to display both expected SHA256 fingerprints from the
authenticated Juju API without opening a target session. Compare each
OpenSSH trust prompt with the fingerprint for that hop.
The `--print-keys` option cannot be combined with `--direct` or
`--show-command`.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/ssh-server/