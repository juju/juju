// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/utils/ssh"

	"github.com/juju/juju/cmd/modelcmd"
)

var usageSSHSummary = `
Initiates an SSH session or executes a command on a Juju machine.`[1:]

var usageSSHDetails = `
The machine is identified by the <target> argument which is either a 'unit
name' or a 'machine id'. Both are obtained in the output to "juju status". If
'user' is specified then the connection is made to that user account;
otherwise, the default 'ubuntu' account, created by Juju, is used.

The optional command is executed on the remote machine. Any output is sent back
to the user. Screen-based programs require the default of '--pty=true'.

The SSH host keys of the target are verified. The --no-host-key-checks option
can be used to disable these checks. Use of this option is not recommended as
it opens up the possibility of a man-in-the-middle attack.

Examples:
Connect to machine 0:

    juju ssh 0

Connect to machine 1 and run command 'uname -a':

    juju ssh 1 uname -a

Connect to a mysql unit:

    juju ssh mysql/0

Connect to a jenkins unit as user jenkins:

    juju ssh jenkins@jenkins/0

See also: 
    scp`

func newSSHCommand() cmd.Command {
	return modelcmd.Wrap(&sshCommand{})
}

// sshCommand is responsible for launching a ssh shell on a given unit or machine.
type sshCommand struct {
	SSHCommon
}

func (c *sshCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "ssh",
		Args:    "<[user@]target> [command]",
		Purpose: usageSSHSummary,
		Doc:     usageSSHDetails,
	}
}

func (c *sshCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.Errorf("no target name specified")
	}
	c.Target, c.Args = args[0], args[1:]
	return nil
}

// Run resolves c.Target to a machine, to the address of a i
// machine or unit forks ssh passing any arguments provided.
func (c *sshCommand) Run(ctx *cmd.Context) error {
	err := c.initRun()
	if err != nil {
		return errors.Trace(err)
	}
	defer c.cleanupRun()

	target, err := c.resolveTarget(c.Target)
	if err != nil {
		return err
	}

	options, err := c.getSSHOptions(c.pty, target)
	if err != nil {
		return err
	}

	cmd := ssh.Command(target.userHost(), c.Args, options)
	cmd.Stdin = ctx.Stdin
	cmd.Stdout = ctx.Stdout
	cmd.Stderr = ctx.Stderr
	return cmd.Run()
}
