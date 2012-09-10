package main

import (
	"errors"
	"os/exec"
	"os"

	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
)

// SSHCommand is responsible for launchin an ssh shell on a given unit or machine.
type SSHCommand struct {
	EnvName     string
	Target 	string
	Args	[]string
}

func (c *SSHCommand) Info() *cmd.Info {
	return &cmd.Info{"ssh", "", "launch an ssh shell on a given unit or machine", ""}
}

func (c *SSHCommand) Init(f *gnuflag.FlagSet, args []string) error {
	addEnvironFlags(&c.EnvName, f)
	if err := f.Parse(true, args); err != nil {
		return err
	}
	args = f.Args()
	switch len(args) {
	case 0: 
		return errors.New("no service name specified")
	default:
		c.Args = args[1:]
		fallthrough
	case 1:
		c.Target = args[0]
	}
	return nil
}

// Run resolves c.Target to a machine, or host of a unit and
// forks ssh with c.Args, if provided.
// ports that were also explicitly marked by units as open.
func (c *SSHCommand) Run(_ *cmd.Context) error {
	conn, err := juju.NewConnFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer conn.Close()
	host, err := c.hostFromTarget()
	
	cmd := 
}
