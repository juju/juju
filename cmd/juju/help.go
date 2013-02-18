package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
)

type HelpCommand struct {
	Subcommand string
	Parent     *cmd.SuperCommand
}

const helpDoc = `
See also: topics
`

var helpTopics = map[string]string{
	"basics": help_basics,
}

func (c *HelpCommand) Info() *cmd.Info {
	return &cmd.Info{
		"help", "[topic]", "show help on a command or other topic", helpDoc,
	}
}

func (c *HelpCommand) Init(f *gnuflag.FlagSet, args []string) error {
	// This flag parsing is primarily to get the --help option.
	if err := f.Parse(true, args); err != nil {
		return err
	}
	args = f.Args()
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
		fmt.Fprintf(ctx.Stderr, helpTopics["basics"])
	} else {
		if command, found := c.Parent.GetCommand(c.Subcommand); found {
			// TODO: Why Stderr and not Stdout?
			// FIXME: this is bollocks
			info := command.Info()
			f := gnuflag.NewFlagSet(info.Name, gnuflag.ContinueOnError)
			ctx.Stderr.Write(info.Help(f))
		} else {
			// Look in the topics
			if topic, found := helpTopics[c.Subcommand]; found {
				fmt.Fprintf(ctx.Stderr, topic)
			} else {
				fmt.Fprintf(ctx.Stderr, "Unknown command or topic for %s\n", c.Subcommand)
			}
		}
	}
	return nil
}
