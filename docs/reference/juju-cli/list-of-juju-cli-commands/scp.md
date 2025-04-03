(command-juju-scp)=
# `juju scp`

```
Usage: juju scp [options] <source> <destination>

Summary:
Securely transfer files within a model.

Global Options:
--debug  (= false)
    equivalent to --show-log --logging-config=<root>=DEBUG
-h, --help  (= false)
    Show help on a command or other topic.
--logging-config (= "")
    specify log levels for modules
--quiet  (= false)
    show no informational output
--show-log  (= false)
    if set, write the log file to stderr
--verbose  (= false)
    show more verbose output

Command Options:
--container (= "")
    the container name of the target pod
-m, --model (= "")
    Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>
--no-host-key-checks  (= false)
    Skip host key checking (INSECURE)
--proxy  (= false)
    Proxy through the API server
--remote  (= false)
    Target on the workload or operator pod (k8s-only)

Details:
Transfer files to, from and between Juju machine(s), unit(s) and the
Juju client.

The basic syntax for the command requires the location of 1 or more source
files or directories and their intended destination:

    <source> <destination>

The <source> and <destination> arguments may either be a path to a local file
or a remote location. Here is a fuller syntax diagram:

    # <source>                 <destination>
    [[<user>@]<target>:]<path> [<user>@]<target>:[<path>]

<user> is a user account that exists on the remote host. Juju defaults to the
"ubuntu" user when this is omitted.

<target> may be either a unit or machine. Units are specified in form
'<application-name>/<n>', where '<n>' is either the unit number or the value
"leader" when targeting the leader unit for an application e.g. postgresql/0 or
haproxy/leader. Machines are specified in form '<n>', e.g. 0 or 12. The units
and machines in your model can be obtained from the output of "juju status".

<path> is a file path. Local relative paths are resolved relative to the
current working directory. Remote relative paths are resolved relative to the
home directory of the remote user account.


Providing arguments directly to scp

Send arguments directly to the underlying scp utility for full control by
adding two hyphens to the argument list and adding arguments to the right
(-- <arg> [...]). Common arguments to scp include

 - "-r" recursively copy files from a directory
 - "-3" use the client as a proxy for transfers between machines
 - "-C" enable SSH compression


Transfers between machines

Machines do not have SSH connectivity to each other by default. Within a Juju
model, all communication is facilitated by the Juju controller. To transfer
files between machines, you can use the -3 option to scp, e.g. add "-- -3"
to the command-line arguments.


Security considerations

To enable transfers to/from machines that do not have internet access, you can use
the Juju controller as a proxy with the --proxy option.

The SSH host keys of the target are verified by default. To disable this, add
 --no-host-key-checks option. Using this option is strongly discouraged.


Examples:

    # Copy the config of a Charmed Kubernetes cluster to ~/.kube/config
    juju scp kubernetes-master/0:config ~/.kube/config

    # Copy file /var/log/syslog from machine 2 to the client's
    # current working directory:
    juju scp 2:/var/log/syslog .

    # Recursively copy the /var/log/mongodb directory from the
    # mongodb/0 unit to the client's local remote-logs directory:
    juju scp -- -r mongodb/0:/var/log/mongodb/ remote-logs

    # Copy foo.txt from the client's current working directory to a
    # the apache2/1 unit model "prod" (-m prod). Proxy the SSH connection
    # through the controller (--proxy) and enable compression (-- -C):
    juju scp -m prod --proxy -- -C foo.txt apache2/1:

    # Copy multiple files from the client's current working directory to
    # the /home/ubuntu directory of machine 2:
    juju scp file1 file2 2:

    # Copy multiple files from machine 3 as user "bob" to the client's
    # current working directory:
    juju scp bob@3:'file1 file2' .

    # Copy file.dat from machine 0 to the machine hosting unit foo/0
    # (-- -3):
    juju scp -- -3 0:file.dat foo/0:

    # Copy a file ('chunks-inspect') from localhost to /loki directory
    # in a specific container in a juju unit running in Kubernetes:
    juju scp --container loki chunks-inspect loki-k8s/0:/loki

See also:
	ssh
```