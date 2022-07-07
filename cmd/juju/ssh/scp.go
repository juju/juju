// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/retry"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/model"
	jujussh "github.com/juju/juju/network/ssh"
)

var usageSCPSummary = `
Securely transfer files within a model.`[1:]

var usageSCPDetails = `
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

See also: 
	ssh
`

func NewSCPCommand(hostChecker jujussh.ReachableChecker, retryStrategy retry.CallArgs) cmd.Command {
	c := new(scpCommand)
	c.hostChecker = hostChecker
	c.retryStrategy = retryStrategy
	return modelcmd.Wrap(c)
}

// scpCommand is responsible for launching a scp command to copy files to/from remote machine(s)
type scpCommand struct {
	modelcmd.ModelCommandBase

	modelType model.ModelType

	sshMachine
	sshContainer

	provider sshProvider

	hostChecker jujussh.ReachableChecker

	retryStrategy retry.CallArgs
}

func (c *scpCommand) SetFlags(f *gnuflag.FlagSet) {
	c.sshMachine.SetFlags(f)
	c.sshContainer.SetFlags(f)
}

func (c *scpCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "scp",
		Args:    "<source> <destination>",
		Purpose: usageSCPSummary,
		Doc:     usageSCPDetails,
	})
}

func (c *scpCommand) Init(args []string) (err error) {
	if len(args) < 2 {
		return errors.Errorf("at least two arguments required")
	}
	if c.modelType, err = c.ModelType(); err != nil {
		return err
	}
	if c.modelType == model.CAAS {
		c.provider = &c.sshContainer
	} else {
		c.provider = &c.sshMachine
	}

	c.provider.setArgs(args)
	c.provider.setHostChecker(c.hostChecker)
	c.provider.setRetryStrategy(c.retryStrategy)
	return nil
}

// Run resolves c.Target to a machine, or host of a unit and
// forks ssh with c.Args, if provided.
func (c *scpCommand) Run(ctx *cmd.Context) error {
	if err := c.provider.initRun(&c.ModelCommandBase); err != nil {
		return errors.Trace(err)
	}
	defer c.provider.cleanupRun()
	return c.provider.copy(ctx)
}
