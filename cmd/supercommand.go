package cmd

import (
	"bytes"
	"fmt"
	"launchpad.net/gnuflag"
	"sort"
	"strings"
)

// SuperCommand is a Command that selects a subcommand and assumes its
// properties; any command line arguments that were not used in selecting
// the subcommand are passed down to it, and to Run a SuperCommand is to run
// its selected subcommand.
type SuperCommand struct {
	CommandBase
	Name    string
	Purpose string
	Doc     string
	Log     *Log
	subcmds map[string]Command
	flags   *gnuflag.FlagSet
	subcmd  Command
	help    Command
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
	if _, found := c.subcmds[name]; found {
		panic(fmt.Sprintf("command already registered: %s", name))
	}
	c.subcmds[name] = subcmd
	if name == "help" {
		c.help = subcmd
	}
}

// DescribeCommands returns a short description of each registered subcommand.
func (c *SuperCommand) DescribeCommands(simple bool) string {
	var lineFormat = "    %-*s - %s"
	var outputFormat = "commands:\n%s"
	if simple {
		lineFormat = "%-*s  %s"
		outputFormat = "%s"
	}
	cmds := make([]string, len(c.subcmds))
	if len(cmds) == 0 {
		return ""
	}
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

// Supercommand help becomes more interesting if there is a help command.
func (c *SuperCommand) Help(i *Info, f *gnuflag.FlagSet) []byte {
	if c.help == nil {
		return c.CommandBase.Help(i, f)
	}
	// Now we can do special magic, since we know that if there is a
	// subcommand specified that it is valid, and if there is no subcommand,
	// then we just do the default help.
	init_args := []string{}
	if c.subcmd != nil {
		init_args = []string{c.subcmd.Info().Name}
	}
	ctx := &Context{Stdout: &bytes.Buffer{}}
	c.help.Init(init_args) // need to initialize the topics
	c.help.Run(ctx)
	return ctx.Stdout.(*bytes.Buffer).Bytes()
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
	if cmds := c.DescribeCommands(false); cmds != "" {
		docParts = append(docParts, cmds)
	}
	return &Info{
		Name:    c.Name,
		Args:    "<command> ...",
		Purpose: c.Purpose,
		Doc:     strings.Join(docParts, "\n\n"),
	}
}

// GetCommand looks up the subcommand map for the command identified by the
// name parameter.  Returns (Command, true) if found, and (nil, false) if not.
func (c *SuperCommand) GetCommand(name string) (Command, bool) {
	command, found := c.subcmds[name]
	return command, found
}

// SetFlags adds the options that apply to all commands, particularly those
// due to logging.
func (c *SuperCommand) SetFlags(f *gnuflag.FlagSet) {
	if c.Log != nil {
		c.Log.AddFlags(f)
	}
	// If we have a help command, register the flag for help.  This stops the
	// gnuflag code from returning the ErrHelp error.
	if c.help != nil {

	}
	c.flags = f
}

// Init initializes the command for running.
func (c *SuperCommand) Init(args []string) error {
	if len(args) == 0 {
		// If there are no args specified, and we have a help command, call
		// that with no args.
		if c.help == nil {
			return fmt.Errorf("no command specified")
		} else {
			c.subcmd = c.help
		}
	} else {
		found := false
		if c.subcmd, found = c.subcmds[args[0]]; !found {
			return fmt.Errorf("unrecognized command: %s %s", c.Info().Name, args[0])
		}
		args = args[1:]
	}
	c.subcmd.SetFlags(c.flags)
	if err := c.flags.Parse(true, args); err != nil {
		return err
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
