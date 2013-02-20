package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"sort"
	"strings"
)

type topic struct {
	short string
	long  func() string
}

type HelpCommand struct {
	Subcommand string
	Parent     *cmd.SuperCommand
	topics     map[string]topic
}

const helpDoc = `
See also: topics
`

func echo(s string) func() string {
	return func() string { return s }
}

func (c *HelpCommand) make_topics() {
	c.topics = make(map[string]topic)
	c.topics["basics"] = topic{short: "Basic commands", long: echo(help_basics)}
	c.topics["commands"] = topic{short: "Basic help for all commands",
		long: func() string { return c.commands() }}
	c.topics["global-options"] = topic{short: "Options that control how Juju runs",
		long: func() string { return c.global_options() }}
	c.topics["topics"] = topic{short: "Topic list",
		long: func() string { return c.topic_list() }}
}

func (c *HelpCommand) commands() string {
	return c.Parent.DescribeCommands(true)
}

func (c *HelpCommand) global_options() string {
	return "todo: global_options"
}

func (c *HelpCommand) topic_list() string {
	topics := make([]string, len(c.topics))
	i := 0
	longest := 0
	for name := range c.topics {
		if len(name) > longest {
			longest = len(name)
		}
		topics[i] = name
		i++
	}
	sort.Strings(topics)
	for i, name := range topics {
		short_help := c.topics[name].short
		topics[i] = fmt.Sprintf("%-*s  %s", longest, name, short_help)
	}
	return fmt.Sprintf("%s", strings.Join(topics, "\n"))
}

func (c *HelpCommand) get_topic_text(name string) (string, bool) {
	if topic, found := c.topics[name]; found {
		return strings.TrimSpace(topic.long()), true
	}
	return "", false
}

func (c *HelpCommand) Info() *cmd.Info {
	return cmd.NewInfo(
		"help", "[topic]", "show help on a command or other topic", helpDoc,
	)
}

func (c *HelpCommand) SetFlags(f *gnuflag.FlagSet) {}

func (c *HelpCommand) Init(args []string) error {
	if c.topics == nil {
		c.make_topics()
	}
	// This flag parsing is primarily to get the --help option.
	switch len(args) {
	case 0:
		// do nothing
	case 1:
		c.Subcommand = args[0]
	default:
		return fmt.Errorf("extra argument to command help: %q", args[1])
	}
	return nil
}

func (c *HelpCommand) Run(ctx *cmd.Context) error {
	// Is there a reason why help was written to stderr instead of stdout?
	if c.Subcommand == "" {
		text, _ := c.get_topic_text("basics")
		fmt.Fprintf(ctx.Stderr, "%s\n", text)
	} else {
		if command, found := c.Parent.GetCommand(c.Subcommand); found {
			// TODO: Why Stderr and not Stdout?
			// FIXME: this is bollocks
			info := command.Info()
			f := gnuflag.NewFlagSet(info.Name, gnuflag.ContinueOnError)
			command.SetFlags(f)
			ctx.Stderr.Write(info.Help(f))
		} else {
			// Look in the topics
			if text, found := c.get_topic_text(c.Subcommand); found {
				fmt.Fprintf(ctx.Stderr, "%s\n", text)
			} else {
				fmt.Fprintf(ctx.Stderr, "Unknown command or topic for %s\n", c.Subcommand)
			}
		}
	}
	return nil
}
