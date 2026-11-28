(command-juju-scp)=
# `juju scp`
> See also: [ssh](#command-juju-ssh)

## Summary
Securely transfer files within a model.

## Usage
```text
juju scp [options] <source> <destination>
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--container` |  | the container name of the target pod |
| `--direct` | false | Use the legacy machine SSH or Kubernetes API route instead of the SSH jump server |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `--no-host-key-checks` | false | Skip host key checking (INSECURE) |
| `--proxy` | &lt;auto&gt; | Proxy through the controller machine's OpenSSH server (requires `--direct`; defaults to the model's `proxy-ssh` setting) |
| `--show-command` | false | Print the OpenSSH command for the jump route instead of running it |
| `--ssh-key` |  | Private key to offer to the controller's SSH jump server |

## Examples

Copy the config of a Charmed Kubernetes cluster to `~/.kube/config`:

    juju scp kubernetes-master/0:config ~/.kube/config

Copy file `/var/log/syslog` from machine `2` to the client's
current working directory:

    juju scp 2:/var/log/syslog .

Recursively copy the `/var/log/mongodb` directory from the
`mongodb/0` unit to the client's local `remote-logs` directory:

    juju scp -- -r mongodb/0:/var/log/mongodb/ remote-logs

Copy `foo.txt` to the `apache2/1` machine unit in model `prod` over the
default jump route. Pass `-C` to scp to enable compression:

    juju scp -m prod -- -C foo.txt apache2/1:

Copy multiple files from the client's current working directory to
the `/home/ubuntu` directory of machine `2`:

    juju scp file1 file2 2:

In direct mode, download a file from machine `3` as remote user `bob`:

    juju scp --direct bob@3:/home/bob/report.txt .

Copy `file.dat` from machine 0 to the machine hosting unit `foo/0`
(`-- -3`):

    juju scp -- -3 0:file.dat foo/0:

Copy a file (`chunks-inspect`) from `localhost` to the `/loki` directory
in a specific container in a Juju unit running in Kubernetes:

    juju scp --container loki chunks-inspect loki-k8s/0:/loki


## Details

By default, this command copies through the controller's SSH jump server.
Use a local path or a remote path of the form `<target>:<path>`. Choose the
target from `juju status`; `<application>/leader` selects the leader unit.

### Machine models

Use a top-level machine number or a machine-backed unit. The client runs
OpenSSH scp; specify one or more sources followed by the destination.
The machine login is `ubuntu` on the jump route. To pass options to scp,
put them after `--`, before the paths. Use `-r` for a directory, `-C`
for compression or `-3` to copy between two remote machines through your
client. By default the client verifies both jump and virtual host keys. Use
`--show-command` to print the OpenSSH command for a machine copy instead
of running it. The printed command uses OpenSSH's own host-key configuration;
compare trust prompts with `juju ssh --print-keys <target>`.

### Kubernetes models

Specify exactly one local path and one path in a unit, in either order.
The jump route streams an archive and requires `tar` in the selected
container. The default container is `charm`; use `--container` to choose
another in the same pod. Pod-to-pod copies, scp options and
`--show-command` are not supported on this route.

### Direct mode

For an older controller without jump support, specify `--direct`; the
client does not switch automatically. A machine copy then uses ordinary SSH,
where `user@target:path` can select a different account. A Kubernetes copy
uses the Kubernetes API. Neither direct route supports `--show-command`.

On machine models, `--direct --proxy` copies through the controller
machine's OpenSSH server (port 22 by default). If you omit `--proxy` in
direct mode, Juju uses the model's `proxy-ssh` setting. To bypass the
proxy when your client can reach the machine directly, use `--direct --proxy=false`.

On client build `4.1-beta3-048d091` the proxy hop fails, and
`--proxy=false` does not override a model with `proxy-ssh=true`.
Use the default jump route (omit both flags) if the controller supports it.
If you need the proxy hop, use a client with the fix for Juju issue #23913.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/ssh-server/