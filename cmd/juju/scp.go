package main

import (
	"errors"
	"os/exec"
	"strings"

	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
)

// SCPCommand is responsible for launching a scp command to copy files to/from remote machine(s)
type SCPCommand struct {
	SSHCommon
}

func (c *SCPCommand) Info() *cmd.Info {
	return &cmd.Info{"scp", "", "launch a scp command to copy files to/from remote machine(s)", ""}
}

func (c *SCPCommand) Init(f *gnuflag.FlagSet, args []string) error {
	addEnvironFlags(&c.EnvName, f)
	if err := f.Parse(true, args); err != nil {
		return err
	}
	switch len(f.Args()) {
	case 0, 1:
		return errors.New("at least two arguments required")
	default:
		c.Args = f.Args()
	}
	return nil
}

// Run resolves c.Target to a machine, or host of a unit and
// forks ssh with c.Args, if provided.
func (c *SCPCommand) Run(ctx *cmd.Context) error {
	var err error
	c.Conn, err = juju.NewConnFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer c.Close()

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

	args := []string{"-o", "StrictHostKeyChecking no", "-o", "PasswordAuthentication no"}
	args = append(args, c.Args...)
	cmd := exec.Command("scp", args...)
	cmd.Stdin = ctx.Stdin
	cmd.Stdout = ctx.Stdout
	cmd.Stderr = ctx.Stderr
	return cmd.Run()
}
