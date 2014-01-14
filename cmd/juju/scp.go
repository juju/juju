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
Lauch an scp command to copy files. <to> and <from> are either local
file paths or remote locations of the form <target>:<path>, where
<target> can be either a machine id as listed by "juju status" in the
"machines" section or a unit name as listed in the "services" section.

Examples

Copy a single file from machine 2 to the local machine:

    juju scp 2:/var/log/syslog .

Recursively copy the directory /var/log/mongodb/ on the first mongodb
server to the local directory remote-logs:

    juju scp -- -r mongodb/0:/var/log/mongodb/ remote-logs/

Copy a local file to the second apache unit of the environment "testing":

    juju scp -e testing foo.txt apache2/1:
`

func (c *SCPCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "scp",
		Args:    "[-- scp-option...] <from> <to>",
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

	// translate arguments in the form 0:/somepath or service/0:/somepath into
	// ubuntu@machine:/somepath so they can be presented to scp.
	for i := range c.Args {
		// BUG(dfc) This will not work for IPv6 addresses like 2001:db8::1:2:/somepath.
		if v := strings.SplitN(c.Args[i], ":", 2); len(v) > 1 {
			host, err := c.hostFromTarget(v[0])
			if err != nil {
				return err
			}
			c.Args[i] = "ubuntu@" + host + ":" + v[1]
		}
	}
	return ssh.Copy(c.Args[0], c.Args[1], nil)
}
