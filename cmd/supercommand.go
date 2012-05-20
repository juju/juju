package cmd

import (
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
	Name    string
	Purpose string
	Doc     string
	Log     *Log
	subcmds map[string]Command
	subcmd  Command
}

// Register makes a subcommand available for use on the command line.
func (c *SuperCommand) Register(subcmd Command) {
	if c.subcmds == nil {
		c.subcmds = make(map[string]Command)
	}
	name := subcmd.Info().Name
	_, found := c.subcmds[name]
	if found {
		panic(fmt.Sprintf("command already registered: %s", name))
	}
	c.subcmds[name] = subcmd
}

// describeCommands returns a short description of each registered subcommand.
func (c *SuperCommand) describeCommands() string {
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
		purpose := c.subcmds[name].Info().Purpose
		cmds[i] = fmt.Sprintf("    %-*s - %s", longest, name, purpose)
	}
	return fmt.Sprintf("commands:\n%s", strings.Join(cmds, "\n"))
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
	if cmds := c.describeCommands(); cmds != "" {
		docParts = append(docParts, cmds)
	}
	return &Info{c.Name, "<command> ...", c.Purpose, strings.Join(docParts, "\n\n")}
}

// Init initializes the command for running.
func (c *SuperCommand) Init(f *gnuflag.FlagSet, args []string) error {
	if c.Log != nil {
		c.Log.AddFlags(f)
	}
	if err := f.Parse(false, args); err != nil {
		return err
	}
	subargs := f.Args()
	if len(subargs) == 0 {
		return fmt.Errorf("no command specified")
	}
	found := false
	if c.subcmd, found = c.subcmds[subargs[0]]; !found {
		return fmt.Errorf("unrecognized command: %s %s", c.Info().Name, subargs[0])
	}
	return c.subcmd.Init(f, subargs[1:])
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
