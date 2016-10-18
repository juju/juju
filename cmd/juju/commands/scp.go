// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"net"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/utils/ssh"

	"github.com/juju/juju/cmd/modelcmd"
)

var usageSCPSummary = `
Transfers files to/from a Juju machine.`[1:]

var usageSCPDetails = `
The source or destination arguments may either be a local path or a remote
location. The syntax for a remote location is:

    [<user>@]<target>:[<path>]

If the user is not specified, "ubuntu" is used. If <path> is not specified, it
defaults to the home directory of the remote user account.

The <target> may be either a 'unit name' or a 'machine id'. These can be
obtained from the output of "juju status".

Options specific to scp can be provided after a "--". Refer to the scp(1) man
page for an explanation of those options. The "-r" option to recursively copy a
directory is particularly useful.

The SSH host keys of the target are verified. The --no-host-key-checks option
can be used to disable these checks. Use of this option is not recommended as
it opens up the possibility of a man-in-the-middle attack.

Examples:

Copy file /var/log/syslog from machine 2 to the client's current working
directory:

    juju scp 2:/var/log/syslog .

Recursively copy the /var/log/mongodb directory from a mongodb unit to the
client's local remote-logs directory:

    juju scp -- -r mongodb/0:/var/log/mongodb/ remote-logs

Copy foo.txt from the client's current working directory to an apache2 unit of
model "prod". Proxy the SSH connection through the controller and turn on scp
compression:

    juju scp -m prod --proxy -- -C foo.txt apache2/1:

Copy multiple files from the client's current working directory to machine 2:

    juju scp file1 file2 2:

Copy multiple files from the bob user account on machine 3 to the client's
current working directory:

    juju scp bob@3:'file1 file2' .

Copy file.dat from machine 0 to the machine hosting unit foo/0 (-3
causes the transfer to be made via the client):

    juju scp -- -3 0:file.dat foo/0:

See also: 
    ssh`

func newSCPCommand() cmd.Command {
	return modelcmd.Wrap(&scpCommand{})
}

// scpCommand is responsible for launching a scp command to copy files to/from remote machine(s)
type scpCommand struct {
	SSHCommon
}

func (c *scpCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "scp",
		Args:    "<source> <destination>",
		Purpose: usageSCPSummary,
		Doc:     usageSCPDetails,
	}
}

func (c *scpCommand) Init(args []string) error {
	if len(args) < 2 {
		return errors.Errorf("at least two arguments required")
	}
	c.Args = args
	return nil
}

// Run resolves c.Target to a machine, or host of a unit and
// forks ssh with c.Args, if provided.
func (c *scpCommand) Run(ctx *cmd.Context) error {
	err := c.initRun()
	if err != nil {
		return errors.Trace(err)
	}
	defer c.cleanupRun()

	args, targets, err := expandArgs(c.Args, c.resolveTarget)
	if err != nil {
		return err
	}

	options, err := c.getSSHOptions(false, targets...)
	if err != nil {
		return err
	}

	return ssh.Copy(args, options)
}

// expandArgs takes a list of arguments and looks for ones in the form of
// 0:some/path or application/0:some/path, and translates them into
// ubuntu@machine:some/path so they can be passed as arguments to scp, and pass
// the rest verbatim on to scp
func expandArgs(args []string, resolveTarget func(string) (*resolvedTarget, error)) (
	[]string, []*resolvedTarget, error,
) {
	outArgs := make([]string, len(args))
	var targets []*resolvedTarget
	for i, arg := range args {
		v := strings.SplitN(arg, ":", 2)
		if strings.HasPrefix(arg, "-") || len(v) <= 1 {
			// Can't be an interesting target, so just pass it along
			outArgs[i] = arg
			continue
		}

		target, err := resolveTarget(v[0])
		if err != nil {
			return nil, nil, err
		}
		arg := net.JoinHostPort(target.host, v[1])
		if target.user != "" {
			arg = target.user + "@" + arg
		}
		outArgs[i] = arg

		targets = append(targets, target)
	}
	return outArgs, targets, nil
}
