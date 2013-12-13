// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"strconv"

	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/cmd"
)

type DebugLogCommand struct {
	cmd.CommandBase
	// The debug log command simply invokes juju ssh with the required arguments.
	sshCmd cmd.Command
	lines  linesValue
}

// defaultLineCount is the default number of lines to
// display, from the end of the consolidated log.
const defaultLineCount = 10

// linesValue implements gnuflag.Value, and represents
// a -n/--lines flag value compatible with "tail".
//
// A negative value (-K) corresponds to --lines=K,
// i.e. the last K lines; a positive value (+K)
// corresponds to --lines=+K, i.e. from line K onwards.
type linesValue int

func (v *linesValue) String() string {
	if *v > 0 {
		return fmt.Sprintf("+%d", *v)
	}
	return fmt.Sprint(-*v)
}

func (v *linesValue) Set(value string) error {
	if len(value) > 0 {
		sign := -1
		if value[0] == '+' {
			value = value[1:]
			sign = 1
		}
		n, err := strconv.ParseInt(value, 10, 0)
		if err == nil && n > 0 {
			*v = linesValue(sign * int(n))
			return nil
		}
		// err is quite verbose, and doesn't convey
		// any additional useful information.
	}
	return fmt.Errorf("invalid number of lines")
}

const debuglogDoc = `
Launch an ssh shell on the state server machine and tail the consolidated log file.
The consolidated log file contains log messages from all nodes in the environment.
`

func (c *DebugLogCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "debug-log",
		Purpose: "display the consolidated log file",
		Doc:     debuglogDoc,
	}
}

func (c *DebugLogCommand) SetFlags(f *gnuflag.FlagSet) {
	c.sshCmd.SetFlags(f)

	c.lines = -defaultLineCount
	f.Var(&c.lines, "n", "output the last K lines; or use -n +K to output lines starting with the Kth")
	f.Var(&c.lines, "lines", "")
}

func (c *DebugLogCommand) AllowInterspersedFlags() bool {
	return true
}

func (c *DebugLogCommand) Init(args []string) error {
	tailcmd := fmt.Sprintf("tail -n %s -f /var/log/juju/all-machines.log", &c.lines)
	args = append([]string{"0"}, args...)
	args = append(args, tailcmd)
	return c.sshCmd.Init(args)
}

// Run uses "juju ssh" to log into the state server node
// and tails the consolidated log file which captures log
// messages from all nodes.
func (c *DebugLogCommand) Run(ctx *cmd.Context) error {
	return c.sshCmd.Run(ctx)
}
