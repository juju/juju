package cmd

import (
	"bytes"
	"fmt"
	"launchpad.net/gnuflag"
	"sort"
	"strings"
)

type topic struct {
	short string
	long  func() string
}

// SuperCommand is a Command that selects a subcommand and assumes its
// properties; any command line arguments that were not used in selecting
// the subcommand are passed down to it, and to Run a SuperCommand is to run
// its selected subcommand.
type SuperCommand struct {
	CommandBase
	Name     string
	Purpose  string
	Doc      string
	Log      *Log
	subcmds  map[string]Command
	flags    *gnuflag.FlagSet
	subcmd   Command
	showHelp bool
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

func (c *SuperCommand) AddHelpTopic(name, short, long string) {
	c.init()
	c.subcmds["help"].(*helpCommand).addTopic(name, short, long)
}

// Register makes a subcommand available for use on the command line. The
// command will be available via its own name, and via any supplied aliases.
func (c *SuperCommand) Register(subcmd Command) {
	c.init()
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

// SetFlags adds the options that apply to all commands, particularly those
// due to logging.
func (c *SuperCommand) SetFlags(f *gnuflag.FlagSet) {
	if c.Log != nil {
		c.Log.AddFlags(f)
	}
	f.BoolVar(&c.showHelp, "h", false, helpPurpose)
	f.BoolVar(&c.showHelp, "help", false, "")

	c.flags = f
}

// Init initializes the command for running.
func (c *SuperCommand) Init(args []string) error {
	c.init()
	if len(args) == 0 {
		c.subcmd = c.subcmds["help"]
		return nil
	}

	found := false
	// Look for the command.
	if c.subcmd, found = c.subcmds[args[0]]; !found {
		return fmt.Errorf("unrecognized command: %s %s", c.Info().Name, args[0])
	}
	args = args[1:]
	c.subcmd.SetFlags(c.flags)
	if err := c.flags.Parse(true, args); err != nil {
		return err
	}
	if c.showHelp {
		return gnuflag.ErrHelp
	}
	return c.subcmd.Init(c.flags.Args())
}

// Run executes the subcommand that was selected in Init.
func (c *SuperCommand) Run(ctx *Context) error {
	if c.Log != nil {
		if err := c.Log.Start(ctx); err != nil {
			return err
		}
	}
	if c.subcmd == nil {
		panic("Run: missing subcommand; Init failed or not called")
	}
	return c.subcmd.Run(ctx)
}

type helpCommand struct {
	CommandBase
	super  *SuperCommand
	topic  string
	topics map[string]topic
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

func (c *helpCommand) addTopic(name, short, long string) {
	if _, found := c.topics[name]; found {
		panic(fmt.Sprintf("help topic already added: %s", name))
	}
	c.topics[name] = topic{short, echo(long)}
}

func (c *helpCommand) globalOptions() string {
	buf := &bytes.Buffer{}
	fmt.Fprintf(buf, `Global Options

These options may be used with any command, and may appear in front of any
command.

`)

	f := gnuflag.NewFlagSet("", gnuflag.ContinueOnError)
	c.super.SetFlags(f)
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
		return fmt.Errorf("extra arguments to command help: %q", args[2:])
	}
	return nil
}

func (c *helpCommand) Run(ctx *Context) error {
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
	if helpcmd, ok := c.super.subcmds[c.topic]; ok {
		info := helpcmd.Info()
		info.Name = fmt.Sprintf("%s %s", c.super.Name, info.Name)
		f := gnuflag.NewFlagSet(info.Name, gnuflag.ContinueOnError)
		helpcmd.SetFlags(f)
		ctx.Stdout.Write(info.Help(f))
		return nil
	}
	topic, ok := c.topics[c.topic]
	if !ok {
		return fmt.Errorf("Unknown command or topic for %s", c.topic)
	}
	fmt.Fprintf(ctx.Stdout, "%s\n", strings.TrimSpace(topic.long()))
	return nil
}
