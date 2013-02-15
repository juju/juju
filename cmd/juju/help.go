package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
)

type HelpCommand struct {
	Subcommand string
}

const helpDoc = `
See also: topics
`

func (c *HelpCommand) Info() *cmd.Info {
	return &cmd.Info{
		"help", "[topic]", "show help on a command or other topic", helpDoc,
	}
}

func (c *HelpCommand) Init(f *gnuflag.FlagSet, args []string) error {
	switch len(args) {
	case 0:
		// do nothing
	case 1:
		c.Subcommand = args[0]
	default:
		return fmt.Errorf("extra argument to command help: %q", args[2])
	}
	return nil
}

func (c *HelpCommand) Run(ctx *cmd.Context) error {
	// Is there a reason why help was written to stderr instead of stdout?
	if c.Subcommand == "" {
		fmt.Fprintln(ctx.Stderr, "emit the help basics topic")
	} else {
		fmt.Fprintf(ctx.Stderr, "emit the help for %s\n", c.Subcommand)
	}
	return nil
}
