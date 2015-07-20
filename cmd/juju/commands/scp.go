// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"
	"net"
	"strings"

	"github.com/juju/cmd"

	"github.com/juju/juju/utils/ssh"
)

// SCPCommand is responsible for launching a scp command to copy files to/from remote machine(s)
type SCPCommand struct {
	SSHCommon
}

const scpDoc = `
Launch an scp command to copy files. Each argument <file1> ... <file2>
is either local file path or remote locations of the form [<user>@]<target>:<path>,
where <target> can be either a machine id as listed by "juju status" in the
"machines" section or a unit name as listed in the "services" section. If a
username is not specified, the username "ubuntu" will be used.

To pass additional flags to "scp", separate "juju scp" from the options with
"--" to prevent Juju from attempting to interpret the flags. This is only
supported if the scp command can be found in the system PATH. Please refer to
the man page of scp(1) for the supported extra arguments.

Examples:

Copy a single file from machine 2 to the local machine:

    juju scp 2:/var/log/syslog .

Copy 2 files from two units to the local backup/ directory, passing -v
to scp as an extra argument:

    juju scp -- -v ubuntu/0:/path/file1 ubuntu/1:/path/file2 backup/

Recursively copy the directory /var/log/mongodb/ on the first mongodb
server to the local directory remote-logs:

    juju scp -- -r mongodb/0:/var/log/mongodb/ remote-logs/

Copy a local file to the second apache unit of the environment "testing":

    juju scp -e testing foo.txt apache2/1:
`

func (c *SCPCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "scp",
		Args:    "<file1> ... <file2> [scp-option...]",
		Purpose: "launch a scp command to copy files to/from remote machine(s)",
		Doc:     scpDoc,
	}
}

func (c *SCPCommand) Init(args []string) error {
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
func (c *SCPCommand) Run(ctx *cmd.Context) error {
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
