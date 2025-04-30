(command-juju-ssh)=
# `juju ssh`
> See also: [scp](#scp)

## Summary
Initiates an SSH session or executes a command on a Juju machine or container.

## Usage
```juju ssh [options] <[user@]target> [openssh options] [command]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--container` |  | the container name of the target pod |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `--no-host-key-checks` | false | Skip host key checking (INSECURE) |
| `--proxy` | false | Proxy through the API server |
| `--pty` | &lt;auto&gt; | Enable pseudo-tty allocation |

## Examples

Connect to machine 0:

    juju ssh 0

Connect to machine 1 and run command 'uname -a':

    juju ssh 1 uname -a

Connect to the leader mysql unit:

    juju ssh mysql/leader

Connect to a specific mysql unit:

    juju ssh mysql/0

Connect to a jenkins unit as user jenkins:

    juju ssh jenkins@jenkins/0

Connect to a mysql unit with an identity not known to juju (ssh option -i):

    juju ssh mysql/0 -i ~/.ssh/my_private_key echo hello

**For k8s charms running the workload in a separate pod:**

Connect to a k8s unit targeting the operator pod by default:

	juju ssh mysql/0
	juju ssh mysql/0 bash

Connect to a k8s unit targeting the workload pod by specifying --remote:

	juju ssh --remote mysql/0

**For k8s charms using the sidecar pattern:**

Connect to a k8s unit targeting the charm container (the default):

	juju ssh snappass/0
	juju ssh --container charm snappass/0

Connect to a k8s unit targeting the redis container:

	juju ssh --container redis snappass/0

Interact with the Pebble instance in the workload container via the charm container:

    juju ssh snappass/0 PEBBLE_SOCKET=/charm/containers/redis/pebble.socket /charm/bin/pebble plan

**For k8s controller:**

Connect to the api server pod:

    juju ssh --container api-server 0

Connect to the mongo db pod:

    juju ssh --container mongodb 0


## Details

The ssh target is identified by the &lt;target&gt; argument which is either a 'unit
name' or a 'machine id'. Both can be obtained by examining the output to "juju
status".

Valid unit identifiers are:
  a standard unit ID, such as mysql/0 or;
  leader syntax of the form &lt;application&gt;/leader, such as mysql/leader.

If 'user' is specified then the connection is made to that user
account; otherwise, the default 'ubuntu' account, created by Juju, is used.

The optional command is executed on the remote machine, and any output is sent
back to the user. If no command is specified, then an interactive shell session
will be initiated if possible.

When "juju ssh" is executed without a terminal attached, e.g. when piping the
output of another command into it, then the default behavior is to not allocate
a pseudo-terminal (pty) for the ssh session; otherwise a pty is allocated. This
behavior can be overridden by explicitly specifying the behavior with
"--pty=true" or "--pty=false".

The SSH host keys of the target are verified. The --no-host-key-checks option
can be used to disable these checks. Use of this option is not recommended as
it opens up the possibility of a man-in-the-middle attack.

The default identity known to Juju and used by this command is ~/.ssh/id_ed25519

Options can be passed to the local OpenSSH client (ssh) on platforms
where it is available. This is done by inserting them between the target and
a possible remote command. Refer to the ssh man page for an explanation
of those options.

For k8s charms, the --container argument is used to identity a specific
container in the pod. For charms which run the workload in a separate pod
to that of the charm, the default ssh target is the charm operator pod.
The workload pod may be specified using the --remote argument.