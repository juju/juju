package cmd

import (
	"fmt"
	"launchpad.net/gnuflag"
	"sort"
	"strings"
)

// SuperCommand XXXXXXXXXXXXXX
type SuperCommand struct {
	Name    string
	Purpose string
	Doc     string
	LogFile string
	Verbose bool
	Debug   bool
	subcmds map[string]Command
	subcmd  Command
}

// NewSuperCommand returns an initialized SuperCommand.
func NewSuperCommand(name, purpose, doc string) *SuperCommand {
	return &SuperCommand{
		subcmds: make(map[string]Command),
		Name:    name,
		Purpose: purpose,
		Doc:     doc,
	}
}

// Register makes a subcommand available for use on the command line.
func (c *SuperCommand) Register(subcmd Command) {
	name := subcmd.Info().Name
	_, found := c.subcmds[name]
	if found {
		panic(fmt.Sprintf("command already registered: %s", name))
	}
	c.subcmds[name] = subcmd
}

// DescribeCommands returns a short description of each registered subcommand.
func (c *SuperCommand) DescribeCommands() string {
	cmds := make([]string, len(c.subcmds))
	i := 0
	for name, _ := range c.subcmds {
		purpose := c.subcmds[name].Info().Purpose
		cmds[i] = fmt.Sprintf("    %-12s %s\n", name, purpose)
		i++
	}
	sort.Strings(cmds)
	return fmt.Sprintf("commands:\n%s", strings.Join(cmds, ""))
}

// Info returns a description of the currently selected subcommand, or of the
// SuperCommand itself if no subcommand has been specified.
func (c *SuperCommand) Info() *Info {
	var info *Info
	if c.subcmd != nil {
		info = c.subcmd.Info()
		info.Name = fmt.Sprintf("%s %s", c.Name, info.Name)
		return info
	}
	return &Info{
		c.Name, "<command> [options] ...", c.Purpose,
		fmt.Sprintf("%s\n\n%s", strings.TrimSpace(c.Doc), c.DescribeCommands()),
		false,
	}
}

// Init initializes the command for running.
// subcommand, or with the SuperCommand itself if no subcommand has been
// specified.
func (c *SuperCommand) Init(f *gnuflag.FlagSet, args []string) error {
	f.StringVar(&c.LogFile, "log-file", c.LogFile, "path to write log to")
	f.BoolVar(&c.Verbose, "v", c.Verbose, "if set, log additional messages")
	f.BoolVar(&c.Verbose, "verbose", c.Verbose, "if set, log additional messages")
	f.BoolVar(&c.Debug, "d", c.Debug, "if set, log debugging messages")
	f.BoolVar(&c.Debug, "debug", c.Debug, "if set, log debugging messages")

	if err != f.Parse(false, args); err != nil {
		return err
	}
	subargs := f.Args()
	if len(subargs) == 0 {
		return fmt.Errorf("no command specified")
	}
	found := false
	if c.subcmd, found = c.subcmds[subargs[0]]; !found {
		return fmt.Errorf("unrecognised command: %s %s", c.Info().Name, subargs[0])
	}
	return c.subcmd.Init(f, subargs[1:])
}

// Run executes the subcommand that was selected when Parse was called.
func (c *SuperCommand) Run(ctx *Context) error {
	if err := ctx.InitLog(c.Verbose, c.Debug, c.LogFile); err != nil {
		return err
	}
	if c.subcmd == nil {
		panic("Run: missing subcommand; Parse failed or not called")
	}
	return c.subcmd.Run(ctx)
}
