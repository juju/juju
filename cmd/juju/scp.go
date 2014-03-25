// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"strings"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/utils/ssh"
)

// SCPCommand is responsible for launching a scp command to copy files to/from remote machine(s)
type SCPCommand struct {
	SSHCommon
}

const scpDoc = `
Launch an scp command to copy files. Each argument <file1> ... <file2>
is either local file path or remote locations of the form <target>:<path>,
where <target> can be either a machine id as listed by "juju status" in the
"machines" section or a unit name as listed in the "services" section.
Any extra arguments to scp can be passed after at the end. In case OpenSSH
scp command cannot be found in the system PATH environment variable, this
command is also not available for use. Please refer to the man page of scp(1)
for the supported extra arguments.

Examples:

Copy a single file from machine 2 to the local machine:

    juju scp 2:/var/log/syslog .

Copy 2 files from two units to the local backup/ directory, passing -v
to scp as an extra argument:

    juju scp ubuntu/0:/path/file1 ubuntu/1:/path/file2 backup/ -v

Recursively copy the directory /var/log/mongodb/ on the first mongodb
server to the local directory remote-logs:

    juju scp mongodb/0:/var/log/mongodb/ remote-logs/ -r

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

// Run resolves c.Target to a machine, or host of a unit and
// forks ssh with c.Args, if provided.
func (c *SCPCommand) Run(ctx *cmd.Context) error {
	var err error
	c.apiClient, err = c.initAPIClient()
	if err != nil {
		return err
	}
	defer c.apiClient.Close()

	// Parse all arguments, translating those in the form 0:/somepath
	// or service/0:/somepath into ubuntu@machine:/somepath so they
	// can be given to scp as targets (source(s) and destination(s)),
	// and passing any others that look like extra arguments (starting
	// with "-") verbatim to scp.
	var targets, extraArgs []string
	for i, arg := range c.Args {
		if v := strings.SplitN(arg, ":", 2); len(v) > 1 {
			host, err := c.hostFromTarget(v[0])
			if err != nil {
				return err
			}
			// To ensure this works with IPv6 addresses, we need to
			// wrap the host with \[..\], so the colons inside will be
			// interpreted as part of the address and the last one as
			// separator between host and remote path.
			if strings.Contains(host, ":") {
				host = fmt.Sprintf(`\[%s\]`, host)
			}
			targets = append(targets, "ubuntu@"+host+":"+v[1])
			continue
		}
		if strings.HasPrefix(arg, "-") {
			if i != len(c.Args)-1 {
				return fmt.Errorf("unexpected argument %q; extra arguments must be last", arg)
			}
			extraArgs = append(extraArgs, arg)
		} else {
			// Local path
			targets = append(targets, arg)
		}
	}
	return ssh.Copy(targets, extraArgs, nil)
}
