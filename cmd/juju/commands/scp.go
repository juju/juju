// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"
	"net"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/utils/ssh"

	"github.com/juju/juju/cmd/modelcmd"
)

var usageSCPSummary = `
Transfers files to/from a Juju machine.`[1:]

var usageSCPDetails = `
The machine is identified by the <target> argument which is either a 'unit
name' or a 'machine id'. Both are obtained in the output from ` + "`juju \nstatus`" + `: unit name in the [Units] section and machine id in the [Machines]
section.
If 'user' is specified then the connection is made to that user account;
otherwise, the 'ubuntu' account is used.
A 'file' can be an individual file or a directory. If the latter, you must
use the scp option of '-r'.
'file' can also be multiple files/directories. In the first usage do not
use quoting but in the second usage you must use quoting.
Refer to the scp(1) man page for an explanation of scp options.

Examples:
Copy file /var/log/syslog from machine 2 to the current working directory:

    juju scp 2:/var/log/syslog .

Copy directory /var/log/mongodb, recursively, from a mongodb unit
to local directory remote-logs:

    juju scp -- -r mongodb/0:/var/log/mongodb/ remote-logs

Copy file foo.txt, in verbose mode, from the current working directory to an apache2 unit of model "mymodel":

    juju scp -- -v -m mymodel foo.txt apache2/1:

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
		Args:    "[[user@]host1:]file1 ... [[user@]host2:]file2",
		Purpose: usageSCPSummary,
		Doc:     usageSCPDetails,
	}
}

func (c *scpCommand) Init(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("at least two arguments required")
	}
	c.Args = args
	return nil
}

// expandArgs takes a list of arguments and looks for ones in the form of
// 0:some/path or service/0:some/path, and translates them into
// ubuntu@machine:some/path so they can be passed as arguments to scp, and pass
// the rest verbatim on to scp
func expandArgs(args []string, userHostFromTarget func(string) (string, string, error)) ([]string, error) {
	outArgs := make([]string, len(args))
	for i, arg := range args {
		v := strings.SplitN(arg, ":", 2)
		if strings.HasPrefix(arg, "-") || len(v) <= 1 {
			// Can't be an interesting target, so just pass it along
			outArgs[i] = arg
			continue
		}
		user, host, err := userHostFromTarget(v[0])
		if err != nil {
			return nil, err
		}
		outArgs[i] = user + "@" + net.JoinHostPort(host, v[1])
	}
	return outArgs, nil
}

// Run resolves c.Target to a machine, or host of a unit and
// forks ssh with c.Args, if provided.
func (c *scpCommand) Run(ctx *cmd.Context) error {
	var err error
	c.apiClient, err = c.initAPIClient()
	if err != nil {
		return err
	}
	defer c.apiClient.Close()

	options, err := c.getSSHOptions(false)
	if err != nil {
		return err
	}
	args, err := expandArgs(c.Args, c.userHostFromTarget)
	if err != nil {
		return err
	}
	return ssh.Copy(args, options)
}
