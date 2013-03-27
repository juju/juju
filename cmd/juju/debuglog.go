package main

import (
	"io/ioutil"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
)

type DebugLogCommand struct {
	EnvCommandBase
	Args []string
}

const debuglogDoc = `
Launch an ssh shell on the state server machine and tail the consolidated log file.
The consolidated log file contains log messages from all nodes in the environment.
`

func (c *DebugLogCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "debug-log",
		Args:    "[<ssh args>...]",
		Purpose: "display the consolidated log file",
		Doc:     debuglogDoc,
	}
}

func (c *DebugLogCommand) Init(args []string) error {
	c.Args = append([]string{"0"}, args...)
	c.Args = append(c.Args, "tail -f /var/log/juju/all-machines.log")
	return nil
}

// The debug log command simply invokes juju ssh with the required arguments.
var debugLogSSHCmd cmd.Command = &SSHCommand{}

// Run uses "juju ssh" to log into the state server node
// and tails the consolidated log file which captures log
// messages from all nodes.
func (c *DebugLogCommand) Run(ctx *cmd.Context) error {
	sshCmd := debugLogSSHCmd
	f := gnuflag.NewFlagSet(c.Info().Name, gnuflag.ContinueOnError)
	f.SetOutput(ioutil.Discard)
	sshCmd.SetFlags(f)
	if err := cmd.ParseArgs(sshCmd, f, c.Args); err != nil {
		return err
	}
	if err := sshCmd.Init(f.Args()); err != nil {
		return err
	}
	return sshCmd.Run(ctx)
}
