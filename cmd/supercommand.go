// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"sort"
	"strings"

	"launchpad.net/loggo"

	"launchpad.net/gnuflag"
)

var logger = loggo.GetLogger("juju.cmd")

type topic struct {
	short string
	long  func() string
}

type UnrecognizedCommand struct {
	Name string
}

func (e *UnrecognizedCommand) Error() string {
	return fmt.Sprintf("unrecognized command: %s", e.Name)
}

// MissingCallback defines a function that will be used by the SuperCommand if
// the requested subcommand isn't found.
type MissingCallback func(ctx *Context, subcommand string, args []string) error

// SuperCommandParams provides a way to have default parameter to the
// `NewSuperCommand` call.
type SuperCommandParams struct {
	UsagePrefix     string
	Name            string
	Purpose         string
	Doc             string
	Log             *Log
	MissingCallback MissingCallback
}

// NewSuperCommand creates and initializes a new `SuperCommand`, and returns
// the fully initialized structure.
func NewSuperCommand(params SuperCommandParams) *SuperCommand {
	command := &SuperCommand{
		Name:            params.Name,
		Purpose:         params.Purpose,
		Doc:             params.Doc,
		Log:             params.Log,
		usagePrefix:     params.UsagePrefix,
		missingCallback: params.MissingCallback}
	command.init()
	return command
}

// SuperCommand is a Command that selects a subcommand and assumes its
// properties; any command line arguments that were not used in selecting
// the subcommand are passed down to it, and to Run a SuperCommand is to run
// its selected subcommand.
type SuperCommand struct {
	CommandBase
	Name            string
	Purpose         string
	Doc             string
	Log             *Log
	usagePrefix     string
	subcmds         map[string]Command
	commonflags     *gnuflag.FlagSet
	flags           *gnuflag.FlagSet
	subcmd          Command
	showHelp        bool
	showDescription bool
	showVersion     bool
	missingCallback MissingCallback
}

// IsSuperCommand implements Command.IsSuperCommand
func (c *SuperCommand) IsSuperCommand() bool {
	return true
}

// Because Go doesn't have constructors that initialize the object into a
// ready state.
func (c *SuperCommand) init() {
	if c.subcmds != nil {
		return
	}
	help := &helpCommand{
		super: c,
	}
	help.init()
	c.subcmds = map[string]Command{
		"help": help,
	}
}

// AddHelpTopic adds a new help topic with the description being the short
// param, and the full text being the long param.  The description is shown in
// 'help topics', and the full text is shown when the command 'help <name>' is
// called.
func (c *SuperCommand) AddHelpTopic(name, short, long string) {
	c.subcmds["help"].(*helpCommand).addTopic(name, short, echo(long))
}

// AddHelpTopicCallback adds a new help topic with the description being the
// short param, and the full text being defined by the callback function.
func (c *SuperCommand) AddHelpTopicCallback(name, short string, longCallback func() string) {
	c.subcmds["help"].(*helpCommand).addTopic(name, short, longCallback)
}

// Register makes a subcommand available for use on the command line. The
// command will be available via its own name, and via any supplied aliases.
func (c *SuperCommand) Register(subcmd Command) {
	info := subcmd.Info()
	c.insert(info.Name, subcmd)
	for _, name := range info.Aliases {
		c.insert(name, subcmd)
	}
}

func (c *SuperCommand) insert(name string, subcmd Command) {
	if _, found := c.subcmds[name]; found || name == "help" {
		panic(fmt.Sprintf("command already registered: %s", name))
	}
	c.subcmds[name] = subcmd
}

// describeCommands returns a short description of each registered subcommand.
func (c *SuperCommand) describeCommands(simple bool) string {
	var lineFormat = "    %-*s - %s"
	var outputFormat = "commands:\n%s"
	if simple {
		lineFormat = "%-*s  %s"
		outputFormat = "%s"
	}
	cmds := make([]string, len(c.subcmds))
	i := 0
	longest := 0
	for name := range c.subcmds {
		if len(name) > longest {
			longest = len(name)
		}
		cmds[i] = name
		i++
	}
	sort.Strings(cmds)
	for i, name := range cmds {
		info := c.subcmds[name].Info()
		purpose := info.Purpose
		if name != info.Name {
			purpose = "alias for " + info.Name
		}
		cmds[i] = fmt.Sprintf(lineFormat, longest, name, purpose)
	}
	return fmt.Sprintf(outputFormat, strings.Join(cmds, "\n"))
}

// Info returns a description of the currently selected subcommand, or of the
// SuperCommand itself if no subcommand has been specified.
func (c *SuperCommand) Info() *Info {
	if c.subcmd != nil {
		info := *c.subcmd.Info()
		info.Name = fmt.Sprintf("%s %s", c.Name, info.Name)
		return &info
	}
	docParts := []string{}
	if doc := strings.TrimSpace(c.Doc); doc != "" {
		docParts = append(docParts, doc)
	}
	if cmds := c.describeCommands(false); cmds != "" {
		docParts = append(docParts, cmds)
	}
	return &Info{
		Name:    c.Name,
		Args:    "<command> ...",
		Purpose: c.Purpose,
		Doc:     strings.Join(docParts, "\n\n"),
	}
}

const helpPurpose = "show help on a command or other topic"

// SetCommonFlags creates a new "commonflags" flagset, whose
// flags are shared with the argument f; this enables us to
// add non-global flags to f, which do not carry into subcommands.
func (c *SuperCommand) SetCommonFlags(f *gnuflag.FlagSet) {
	if c.Log != nil {
		c.Log.AddFlags(f)
	}
	f.BoolVar(&c.showHelp, "h", false, helpPurpose)
	f.BoolVar(&c.showHelp, "help", false, "")
	// In the case where we are providing the basis for a plugin,
	// plugins are required to support the --description argument.
	// The Purpose attribute will be printed (if defined), allowing
	// plugins to provide a sensible line of text for 'juju help plugins'.
	f.BoolVar(&c.showDescription, "description", false, "")
	c.commonflags = gnuflag.NewFlagSet(c.Info().Name, gnuflag.ContinueOnError)
	c.commonflags.SetOutput(ioutil.Discard)
	f.VisitAll(func(flag *gnuflag.Flag) {
		c.commonflags.Var(flag.Value, flag.Name, flag.Usage)
	})
}

// SetFlags adds the options that apply to all commands, particularly those
// due to logging.
func (c *SuperCommand) SetFlags(f *gnuflag.FlagSet) {
	c.SetCommonFlags(f)
	// Only flags set by SetCommonFlags are passed on to subcommands.
	// Any flags added below only take effect when no subcommand is
	// specified (e.g. juju --version).
	f.BoolVar(&c.showVersion, "version", false, "Show the version of juju")
	c.flags = f
}

// For a SuperCommand, we want to parse the args with
// allowIntersperse=false. This will mean that the args may contain other
// options that haven't been defined yet, and that only options that relate
// to the SuperCommand itself can come prior to the subcommand name.
func (c *SuperCommand) AllowInterspersedFlags() bool {
	return false
}

// Init initializes the command for running.
func (c *SuperCommand) Init(args []string) error {
	if c.showDescription {
		return CheckEmpty(args)
	}
	if len(args) == 0 {
		c.subcmd = c.subcmds["help"]
		return nil
	}

	found := false
	// Look for the command.
	if c.subcmd, found = c.subcmds[args[0]]; !found {
		if c.missingCallback != nil {
			c.subcmd = &missingCommand{
				callback:  c.missingCallback,
				superName: c.Name,
				name:      args[0],
				args:      args[1:],
			}
			// Yes return here, no Init called on missing Command.
			return nil
		}
		return fmt.Errorf("unrecognized command: %s %s", c.Name, args[0])
	}
	args = args[1:]
	if c.subcmd.IsSuperCommand() {
		f := gnuflag.NewFlagSet(c.Info().Name, gnuflag.ContinueOnError)
		f.SetOutput(ioutil.Discard)
		c.subcmd.SetFlags(f)
	} else {
		c.subcmd.SetFlags(c.commonflags)
	}
	if err := c.commonflags.Parse(c.subcmd.AllowInterspersedFlags(), args); err != nil {
		return err
	}
	args = c.commonflags.Args()
	if c.showHelp {
		// We want to treat help for the command the same way we would if we went "help foo".
		args = []string{c.subcmd.Info().Name}
		c.subcmd = c.subcmds["help"]
	}
	return c.subcmd.Init(args)
}

// Run executes the subcommand that was selected in Init.
func (c *SuperCommand) Run(ctx *Context) error {
	if c.showDescription {
		if c.Purpose != "" {
			fmt.Fprintf(ctx.Stdout, "%s\n", c.Purpose)
		} else {
			fmt.Fprintf(ctx.Stdout, "%s: no description available\n", c.Info().Name)
		}
		return nil
	}
	if c.subcmd == nil {
		panic("Run: missing subcommand; Init failed or not called")
	}
	if c.Log != nil {
		if err := c.Log.Start(ctx); err != nil {
			return err
		}
	}
	err := c.subcmd.Run(ctx)
	if err != nil && err != ErrSilent {
		logger.Errorf("%v", err)
		// Now that this has been logged, don't log again in cmd.Main.
		err = ErrSilent
	} else {
		logger.Infof("command finished")
	}
	return err
}

type missingCommand struct {
	CommandBase
	callback  MissingCallback
	superName string
	name      string
	args      []string
}

// Missing commands only need to supply Info for the interface, but this is
// never called.
func (c *missingCommand) Info() *Info {
	return nil
}

func (c *missingCommand) Run(ctx *Context) error {
	err := c.callback(ctx, c.name, c.args)
	_, isUnrecognized := err.(*UnrecognizedCommand)
	if !isUnrecognized {
		return err
	}
	return &UnrecognizedCommand{c.superName + " " + c.name}
}

type helpCommand struct {
	CommandBase
	super     *SuperCommand
	topic     string
	topicArgs []string
	topics    map[string]topic
}

func (c *helpCommand) init() {
	c.topics = map[string]topic{
		"commands": {
			short: "Basic help for all commands",
			long:  func() string { return c.super.describeCommands(true) },
		},
		"global-options": {
			short: "Options common to all commands",
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

func (c *helpCommand) addTopic(name, short string, long func() string) {
	if _, found := c.topics[name]; found {
		panic(fmt.Sprintf("help topic already added: %s", name))
	}
	c.topics[name] = topic{short, long}
}

func (c *helpCommand) globalOptions() string {
	buf := &bytes.Buffer{}
	fmt.Fprintf(buf, `Global Options

These options may be used with any command, and may appear in front of any
command.

`)

	f := gnuflag.NewFlagSet("", gnuflag.ContinueOnError)
	c.super.SetCommonFlags(f)
	f.SetOutput(buf)
	f.PrintDefaults()
	return buf.String()
}

func (c *helpCommand) topicList() string {
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
		shortHelp := c.topics[name].short
		topics[i] = fmt.Sprintf("%-*s  %s", longest, name, shortHelp)
	}
	return fmt.Sprintf("%s", strings.Join(topics, "\n"))
}

func (c *helpCommand) Info() *Info {
	return &Info{
		Name:    "help",
		Args:    "[topic]",
		Purpose: helpPurpose,
		Doc: `
See also: topics
`,
	}
}

func (c *helpCommand) Init(args []string) error {
	switch len(args) {
	case 0:
	case 1:
		c.topic = args[0]
	default:
		if c.super.missingCallback == nil {
			return fmt.Errorf("extra arguments to command help: %q", args[1:])
		} else {
			c.topic = args[0]
			c.topicArgs = args[1:]
		}
	}
	return nil
}

func (c *helpCommand) Run(ctx *Context) error {
	if c.super.showVersion {
		var v VersionCommand
		v.SetFlags(c.super.flags)
		v.Init(nil)
		return v.Run(ctx)
	}

	// If there is no help topic specified, print basic usage.
	if c.topic == "" {
		if _, ok := c.topics["basics"]; ok {
			c.topic = "basics"
		} else {
			// At this point, "help" is selected as the SuperCommand's
			// sub-command, but we want the info to be printed
			// as if there was nothing selected.
			c.super.subcmd = nil

			info := c.super.Info()
			f := gnuflag.NewFlagSet(info.Name, gnuflag.ContinueOnError)
			c.SetFlags(f)
			ctx.Stdout.Write(info.Help(f))
			return nil
		}
	}
	// If the topic is a registered subcommand, then run the help command with it
	if helpcmd, ok := c.super.subcmds[c.topic]; ok {
		info := helpcmd.Info()
		info.Name = fmt.Sprintf("%s %s", c.super.Name, info.Name)
		if c.super.usagePrefix != "" {
			info.Name = fmt.Sprintf("%s %s", c.super.usagePrefix, info.Name)
		}
		f := gnuflag.NewFlagSet(info.Name, gnuflag.ContinueOnError)
		helpcmd.SetFlags(f)
		ctx.Stdout.Write(info.Help(f))
		return nil
	}
	// Look to see if the topic is a registered topic.
	topic, ok := c.topics[c.topic]
	if ok {
		fmt.Fprintf(ctx.Stdout, "%s\n", strings.TrimSpace(topic.long()))
		return nil
	}
	// If we have a missing callback, call that with --help
	if c.super.missingCallback != nil {
		helpArgs := []string{"--help"}
		if len(c.topicArgs) > 0 {
			helpArgs = append(helpArgs, c.topicArgs...)
		}
		subcmd := &missingCommand{
			callback:  c.super.missingCallback,
			superName: c.super.Name,
			name:      c.topic,
			args:      helpArgs,
		}
		err := subcmd.Run(ctx)
		_, isUnrecognized := err.(*UnrecognizedCommand)
		if !isUnrecognized {
			return err
		}
	}
	return fmt.Errorf("unknown command or topic for %s", c.topic)
}
