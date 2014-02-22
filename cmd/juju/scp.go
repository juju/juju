// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"errors"
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

Examples:

Copy a single file from machine 2 to the local machine:

    juju scp 2:/var/log/syslog .

Copy 2 files from two units to the local backup/ directory:

    juju scp ubuntu/0:/path/file1 ubuntu/1:/path/file2 backup/

Recursively copy the directory /var/log/mongodb/ on the first mongodb
server to the local directory remote-logs:

    juju scp -- -r mongodb/0:/var/log/mongodb/ remote-logs/

Copy a local file to the second apache unit of the environment "testing":

    juju scp -e testing foo.txt apache2/1:
`

func (c *SCPCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "scp",
		Args:    "[-- scp-option...] <file1> ... <file2>",
		Purpose: "launch a scp command to copy files to/from remote machine(s)",
		Doc:     scpDoc,
	}
}

func (c *SCPCommand) Init(args []string) error {
	switch len(args) {
	case 0, 1:
		return errors.New("at least two arguments required")
	default:
		c.Args = args
	}
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
	for _, arg := range c.Args {
		// BUG(dfc) This will not work for IPv6 addresses like 2001:db8::1:2:/somepath.
		if v := strings.SplitN(arg, ":", 2); len(v) > 1 {
			host, err := c.hostFromTarget(v[0])
			if err != nil {
				return err
			}
			targets = append(targets, "ubuntu@"+host+":"+v[1])
			continue
		}
		if strings.HasPrefix(arg, "-") {
			// Allow -- to be specified last, in which case
			// we need to strip it.
			arg = strings.TrimSpace(strings.TrimPrefix(arg, "--"))
			if arg != "" {
				// Extra argument(s).
				extraArgs = append(extraArgs, arg)
			}
		} else {
			// Local path
			targets = append(targets, arg)
		}
	}
	return ssh.Copy(targets, extraArgs, nil)
}
