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
	Name      string
	Purpose   string
	Doc       string
	Log       *Log
	HelpTopic string
	subcmds   map[string]Command
	flags     *gnuflag.FlagSet
	subcmd    Command
	showHelp  bool
	topics    map[string]topic
}

func echo(s string) func() string {
	return func() string { return s }
}

const helpPurpose = "show help on a command or other topic"

func (c *SuperCommand) initializeHelp() {
	c.topics = map[string]topic{
		"commands": {
			short: "Basic help for all commands",
			long:  func() string { return c.describeCommands(true) },
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

func (c *SuperCommand) AddHelpTopic(name, short, long string) {
	if c.topics == nil {
		c.initializeHelp()
	}
	if _, found := c.topics[name]; found || name == "help" {
		panic(fmt.Sprintf("help topic already added: %s", name))
	}
	c.topics[name] = topic{short, echo(long)}
}

// Register makes a subcommand available for use on the command line. The
// command will be available via its own name, and via any supplied aliases.
func (c *SuperCommand) Register(subcmd Command) {
	if c.subcmds == nil {
		c.subcmds = make(map[string]Command)
	}
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

func (c *SuperCommand) addDefaultTopics() {
	if c.topics == nil {
		c.topics = make(map[string]topic)
	}
}

func (c *SuperCommand) globalOptions() string {
	buf := &bytes.Buffer{}
	fmt.Fprintf(buf, `Global Options

These options may be used with any command, and may appear in front of any
command.

`)

	f := gnuflag.NewFlagSet("", gnuflag.ContinueOnError)
	c.SetFlags(f)
	f.SetOutput(buf)
	f.PrintDefaults()
	return buf.String()
}

func (c *SuperCommand) topicList() string {
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

// DescribeCommands returns a short description of each registered subcommand.
func (c *SuperCommand) describeCommands(simple bool) string {
	var lineFormat = "    %-*s - %s"
	var outputFormat = "commands:\n%s"
	if simple {
		lineFormat = "%-*s  %s"
		outputFormat = "%s"
	}
	cmds := make([]string, len(c.subcmds)+1)
	cmds[0] = "help"
	i := 1
	longest := 4 // len("help")
	for name := range c.subcmds {
		if len(name) > longest {
			longest = len(name)
		}
		cmds[i] = name
		i++
	}
	sort.Strings(cmds)
	for i, name := range cmds {
		if name == "help" {
			cmds[i] = fmt.Sprintf(lineFormat, longest, name, helpPurpose)
			continue
		}
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

// SetFlags adds the options that apply to all commands, particularly those
// due to logging.
func (c *SuperCommand) SetFlags(f *gnuflag.FlagSet) {
	if c.Log != nil {
		c.Log.AddFlags(f)
	}
	f.BoolVar(&c.showHelp, "h", false, "")
	f.BoolVar(&c.showHelp, "help", false, helpPurpose)

	c.flags = f
}

func (c *SuperCommand) getTopicText(name string) (string, bool) {
	if topic, found := c.topics[name]; found {
		return strings.TrimSpace(topic.long()), true
	}
	return "", false
}

func (c *SuperCommand) help(ctx *Context) error {
	// If we have a subcommand specified, then show the help for that subcommand.
	if c.subcmd != nil {
		info := c.subcmd.Info()
		info.Name = fmt.Sprintf("%s %s", c.Name, info.Name)
		f := gnuflag.NewFlagSet(info.Name, gnuflag.ContinueOnError)
		c.subcmd.SetFlags(f)
		ctx.Stdout.Write(info.Help(f))
		return nil
	}
	// If we are asked for help on help, then fake up an Info.
	if c.HelpTopic == "help" || c.HelpTopic == "--help" {
		info := Info{
			Name:    c.Name + " help",
			Args:    "[topic]",
			Purpose: helpPurpose,
			Doc: `
See also: topics
`,
		}
		f := gnuflag.NewFlagSet(info.Name, gnuflag.ContinueOnError)
		ctx.Stdout.Write(info.Help(f))
		return nil
	}
	// If there is no help topic specified, print basic usage.
	if c.HelpTopic == "" {
		info := c.Info()
		f := gnuflag.NewFlagSet(info.Name, gnuflag.ContinueOnError)
		c.SetFlags(f)
		ctx.Stdout.Write(info.Help(f))
		return nil
	}
	// If there is a help topic specified, look for that.
	if topic, found := c.topics[c.HelpTopic]; found {
		fmt.Fprintf(ctx.Stdout, "%s\n", strings.TrimSpace(topic.long()))
	} else {
		return fmt.Errorf("Unknown command or topic for %s\n", c.HelpTopic)
	}
	return nil
}

// Init initializes the command for running.
func (c *SuperCommand) Init(args []string) error {
	if c.topics == nil {
		c.initializeHelp()
	}
	if len(args) == 0 {
		c.showHelp = true
		return nil
	}

	found := false
	if args[0] == "help" {
		c.showHelp = true
		switch len(args) {
		case 1:
			// Nothing else to do.
		case 2:
			{
				if c.subcmd, found = c.subcmds[args[1]]; !found {
					// If the requested help isn't a subcommand, treat it as the topic.
					c.HelpTopic = args[1]
				}
			}
		default:
			return fmt.Errorf("extra argument to command help: %q", args[2])
		}
		return nil
	}
	// Not help, so look for the command.
	if c.subcmd, found = c.subcmds[args[0]]; !found {
		return fmt.Errorf("unrecognized command: %s %s", c.Info().Name, args[0])
	}
	args = args[1:]
	c.subcmd.SetFlags(c.flags)
	if err := c.flags.Parse(true, args); err != nil {
		return err
	}
	// If --help was specified, we don't want to initialize the subcommand as
	// it is likely to return  errors.
	if c.showHelp {
		return nil
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
	if c.showHelp {
		return c.help(ctx)
	}
	if c.subcmd == nil {
		panic("Run: missing subcommand; Init failed or not called")
	}
	return c.subcmd.Run(ctx)
}
