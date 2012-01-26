package main

import (
	"fmt"
	"io"
	"launchpad.net/~rogpeppe/juju/gnuflag/flag"
	"sort"
)

type JujuCommand struct {
	Logfile string
	Verbose bool
	subcmds map[string]Command
	subcmd  Command
}

// Ensure Command interface.
var _ Command = (*JujuCommand)(nil)

// NewJujuCommand returns an initialized JujuCommand.
func NewJujuCommand() *JujuCommand {
	return &JujuCommand{subcmds: make(map[string]Command)}
}

// Register makes a subcommand available for use on the command line.
func (c *JujuCommand) Register(subcmd Command) {
	name := subcmd.Info().Name
	_, alreadythere := c.subcmds[name]
	if alreadythere {
		panic(fmt.Sprintf("command already registered: %s", name))
	}
	c.subcmds[name] = subcmd
}

// DescribeCommands will write a short description of each registered subcommand.
func (c *JujuCommand) DescribeCommands(dst io.Writer) {
	names := make([]string, len(c.subcmds))
	i := 0
	for name, _ := range c.subcmds {
		names[i] = name
		i++
	}
	sort.Strings(names)
	fmt.Fprintln(dst, "\ncommands:")
	for _, name := range names {
		fmt.Fprintln(dst, name)
		fmt.Fprintf(dst, "    %s\n", c.subcmds[name].Info().Description)
	}
}

// Info returns a description of the currently selected subcommand, or of the
// juju command itself if no subcommand has been specified.
func (c *JujuCommand) Info() *Info {
	if c.subcmd != nil {
		return c.subcmd.Info()
	}
	return &Info{
		"juju",
		"juju [options] <command> ...",
		"",
		`
juju provides easy, intelligent service orchestration on top of environments
such as OpenStack, Amazon AWS, or bare metal.

https://juju.ubuntu.com/`,
		func(dst io.Writer) { c.DescribeCommands(dst) }}
}

// InitFlagSet prepares a FlagSet for use with the currently selected
// subcommand, or with the juju command itself if no subcommand has been
// specified.
func (c *JujuCommand) InitFlagSet(f *flag.FlagSet) {
	if c.subcmd != nil {
		c.subcmd.InitFlagSet(f)
		return
	}
	f.StringVar(&c.Logfile, "l", "", "path to write log to")
	f.StringVar(&c.Logfile, "log-file", "", "path to write log to")
	f.BoolVar(&c.Verbose, "v", false, "if set, log additional messages")
	f.BoolVar(&c.Verbose, "verbose", false, "if set, log additional messages")
}

// Unconsumed selects the subcommand specified by subargs and uses it to Parse
// any remaining unconsumed command-line arguments.
func (c *JujuCommand) Unconsumed(subargs []string) error {
	if len(subargs) == 0 {
		return fmt.Errorf("no command specified")
	}
	found := false
	if c.subcmd, found = c.subcmds[subargs[0]]; !found {
		return fmt.Errorf("unrecognised command: %s", subargs[0])
	}
	return Parse(c.subcmd, true, subargs[1:])
}

// Run executes the selected subcommand, which depends on Parse having been
// called with the JujuCommand.
func (c *JujuCommand) Run() error {
	return c.subcmd.Run()
}
