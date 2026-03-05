// Copyright 2012-2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENSE file for details.

package cmd

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/juju/gnuflag"
)

type helpCommand struct {
	CommandBase
	super     *SuperCommand
	topic     string
	topicArgs []string
	topics    map[string]topic

	target      *commandReference
	targetSuper *SuperCommand
}

func (c *helpCommand) init() {
	if c.super.FlagKnownAs == "" {
		c.super.FlagKnownAs = "option"
	}
	flagKey := fmt.Sprintf("global-%vs", c.super.FlagKnownAs)
	c.topics = map[string]topic{
		"commands": {
			short: "Basic help for all commands",
			long:  func() string { return c.describeCommands() },
		},
		flagKey: {
			short: fmt.Sprintf("%vs common to all commands", strings.Title(c.super.FlagKnownAs)),
			long:  func() string { return c.globalOptions() },
		},
		"topics": {
			short: "Topic list",
			long:  func() string { return c.topicList() },
		},
	}
}

func echo(s string) func() string {
	return func() string { return s }
}

func (c *helpCommand) addTopic(name, short string, long func() string, aliases ...string) {
	if _, found := c.topics[name]; found {
		panic(fmt.Sprintf("help topic already added: %s", name))
	}
	c.topics[name] = topic{short, long, false}
	for _, alias := range aliases {
		if _, found := c.topics[alias]; found {
			panic(fmt.Sprintf("help topic already added: %s", alias))
		}
		c.topics[alias] = topic{short, long, true}
	}
}

func (c *helpCommand) describeCommands() string {
	commands := c.super.describeCommands()

	// Sort command names, and work out length of the longest one
	cmdNames := make([]string, 0, len(commands))
	longest := 0
	for name := range commands {
		if len(name) > longest {
			longest = len(name)
		}
		cmdNames = append(cmdNames, name)
	}
	sort.Strings(cmdNames)

	var descr string
	for _, name := range cmdNames {
		if len(descr) > 0 {
			descr += "\n"
		}
		purpose := commands[name]
		descr += fmt.Sprintf("%-*s  %s", longest, name, purpose)
	}
	return descr
}

func (c *helpCommand) globalOptions() string {
	buf := &bytes.Buffer{}
	fmt.Fprintf(buf, `Global %vs

These %vs may be used with any command, and may appear in front of any
command.

`, strings.Title(c.super.FlagKnownAs), c.super.FlagKnownAs)

	f := gnuflag.NewFlagSetWithFlagKnownAs("", gnuflag.ContinueOnError, c.super.FlagKnownAs)
	c.super.SetCommonFlags(f)
	f.SetOutput(buf)
	f.PrintDefaults()
	return buf.String()
}

func (c *helpCommand) topicList() string {
	var topics []string
	longest := 0
	for name, topic := range c.topics {
		if topic.alias {
			continue
		}
		if len(name) > longest {
			longest = len(name)
		}
		topics = append(topics, name)
	}
	sort.Strings(topics)
	for i, name := range topics {
		shortHelp := c.topics[name].short
		topics[i] = fmt.Sprintf("%-*s  %s", longest, name, shortHelp)
	}
	return fmt.Sprintf("%s", strings.Join(topics, "\n"))
}

func (c *helpCommand) Info() *Info {
	return &Info{
		Name:        "help",
		Args:        "[topic]",
		FlagKnownAs: c.super.FlagKnownAs,
		Purpose:     helpPurpose,
		Doc: `
See also: topics
`,
	}
}

func (c *helpCommand) Init(args []string) error {
	if c.super.notifyHelp != nil {
		c.super.notifyHelp(args)
	}

	logger.Tracef(context.TODO(), "helpCommand.Init: %#v", args)
	if len(args) == 0 {
		// If there is no help topic specified, print basic usage if it is
		// there.
		if _, ok := c.topics["basics"]; ok {
			c.topic = "basics"
		}
		return nil
	}

	// Before we start walking down the subcommand list, we want to check
	// to see if the first part is there.
	if _, ok := c.super.subcmds[args[0]]; !ok {
		if c.super.missingCallback == nil && len(args) > 1 {
			return fmt.Errorf("extra arguments to command help: %q", args[1:])
		}
		logger.Tracef(context.TODO(), "help not found, setting topic")
		c.topic, c.topicArgs = args[0], args[1:]
		return nil
	}

	c.targetSuper = c.super
	for len(args) > 0 {
		c.topic, args = args[0], args[1:]
		commandRef, ok := c.targetSuper.subcmds[c.topic]
		if !ok {
			return fmt.Errorf("subcommand %q not found", c.topic)
		}
		c.target = &commandRef
		// If there are more args and the target isn't a super command
		// error out.
		logger.Tracef(context.TODO(), "target name: %s", c.target.name)
		if super, ok := c.target.command.(*SuperCommand); ok {
			c.targetSuper = super
		} else if len(args) > 0 {
			return fmt.Errorf("extra arguments to command help: %q", args)
		}
	}
	return nil
}

func (c *helpCommand) getCommandHelp(ctx context.Context, super *SuperCommand, command Command, alias string) []byte {
	info := command.Info()

	if command != super {
		logger.Tracef(ctx, "command not super")
		// If the alias is to a subcommand of another super command
		// the alias string holds the "super sub" name.
		if alias == "" {
			info.Name = fmt.Sprintf("%s %s", super.Name, info.Name)
		} else {
			info.Name = fmt.Sprintf("%s %s", super.Name, alias)
		}
	}
	if super.usagePrefix != "" {
		logger.Tracef(ctx, "adding super prefix")
		info.Name = fmt.Sprintf("%s %s", super.usagePrefix, info.Name)
	}

	flagsAKA := FlagAlias(command, "")
	if flagsAKA == "" {
		flagsAKA = FlagAlias(super, "")
	}
	if flagsAKA == "" {
		flagsAKA = super.FlagKnownAs
	}
	if flagsAKA == "" {
		flagsAKA = FlagAlias(c, "")
	}
	if flagsAKA == "" {
		flagsAKA = FlagAlias(c.super, "")
	}
	if flagsAKA == "" {
		flagsAKA = c.super.FlagKnownAs
	}
	if flagsAKA == "" {
		// For backward compatibility, the default is 'flag'.
		flagsAKA = "flag"
	}
	f := gnuflag.NewFlagSetWithFlagKnownAs(info.Name, gnuflag.ContinueOnError, flagsAKA)
	command.SetFlags(f)

	superf := gnuflag.NewFlagSetWithFlagKnownAs(super.Info().Name, gnuflag.ContinueOnError, flagsAKA)
	super.SetFlags(superf)
	return info.HelpWithSuperFlags(superf, f)
}

func (c *helpCommand) Run(ctx *Context) error {
	if c.super.showVersion {
		v := newVersionCommand(c.super.version, c.super.versionDetail)
		v.SetFlags(c.super.flags)
		err := v.Init(nil)
		if err != nil {
			return err
		}
		return v.Run(ctx)
	}

	// If the topic is a registered subcommand, then run the help command with it
	if c.target != nil {
		_, err := ctx.Stdout.Write(c.getCommandHelp(ctx, c.targetSuper, c.target.command, c.target.alias))
		if err != nil {
			return err
		}
		return nil
	}

	// If there is no help topic specified, print basic usage.
	if c.topic == "" {
		// At this point, "help" is selected as the SuperCommand's
		// current action, but we want the info to be printed
		// as if there was nothing selected.
		c.super.action.command = nil
		_, err := ctx.Stdout.Write(c.getCommandHelp(ctx, c.super, c.super, ""))
		if err != nil {
			return err
		}
		return nil
	}

	// Look to see if the topic is a registered topic.
	topic, ok := c.topics[c.topic]
	if ok {
		_, err := fmt.Fprintf(ctx.Stdout, "%s\n", strings.TrimSpace(topic.long()))
		if err != nil {
			return err
		}
		return nil
	}
	// If we have a missing callback, call that with --help
	if c.super.missingCallback != nil {
		helpArgs := []string{"--help"}
		if len(c.topicArgs) > 0 {
			helpArgs = append(helpArgs, c.topicArgs...)
		}
		command := &missingCommand{
			callback:  c.super.missingCallback,
			superName: c.super.Name,
			name:      c.topic,
			args:      helpArgs,
		}
		err := command.Run(ctx)
		_, isUnrecognized := err.(*UnrecognizedCommand)
		if !isUnrecognized {
			return err
		}
	}
	return fmt.Errorf("unknown command or topic for %s", c.topic)
}
