package main

import (
	"fmt"
	"io"
	"launchpad.net/~rogpeppe/juju/gnuflag/flag"
	"os"
	"sort"
)

// Command should "be implemented by" any subcommand that wants to be dispatched
// to by a JujuCommand.
type Command interface {
	// Return an Info describing name, usage, etc.
	Info() *Info

	// Interpret cmdline args remaining after command name has been consumed
	Parse(args []string) error

	// Actually run the command
	Run() error
}

// JujuCommand handles top-level argument parsing and dispatch to subcommands.
type JujuCommand struct {
	_flag   *flag.FlagSet
	logfile string
	verbose bool
	subcmds map[string]Command
	subcmd  Command
}

// Logfile will return the logfile path specified on the command line, or "".
func (c *JujuCommand) Logfile() string {
	return c.logfile
}

// Verbose will return true if verbose was specified on the command line.
func (c *JujuCommand) Verbose() bool {
	return c.verbose
}

// Initialise (if necessary) and return the FlagSet used by this command
func (c *JujuCommand) flag() *flag.FlagSet {
	if c._flag == nil {
		c._flag = flag.NewFlagSet("juju", flag.ExitOnError)
		c._flag.StringVar(&c.logfile, "l", "", "path to write log to")
		c._flag.StringVar(&c.logfile, "log-file", "", "path to write log to")
		c._flag.BoolVar(&c.verbose, "v", false, "if set, log additional messages")
		c._flag.BoolVar(&c.verbose, "verbose", false, "if set, log additional messages")
		c._flag.Usage = func() { c.Info().PrintUsage() }
	}
	return c._flag
}

// Register will register a subcommand by name (which must not match that of a
// previously-registered subcommand), such that subsequent calls to Parse() will
// dispatch args following "name" to that subcommand for Parse()ing; and
// subsequent calls to Run() will call the subcommand's Run().
func (c *JujuCommand) Register(subcmd Command) error {
	if c.subcmds == nil {
		c.subcmds = make(map[string]Command)
	}
	name := subcmd.Info().Name()
	_, alreadythere := c.subcmds[name]
	if alreadythere {
		return fmt.Errorf("command already registered: %s", name)
	}
	c.subcmds[name] = subcmd
	return nil
}

// Get sorted command names
func (c *JujuCommand) keys() []string {
	keys := make([]string, len(c.subcmds))
	i := 0
	for k, _ := range c.subcmds {
		keys[i] = k
		i++
	}
	sort.Strings(keys)
	return keys
}

// DescCommands will write a short description of each subcommand to dst.
func (c *JujuCommand) DescCommands(dst io.Writer) {
	fmt.Fprintln(dst, "\ncommands:")
	for _, k := range c.keys() {
		fmt.Fprintln(dst, k)
		fmt.Fprintf(dst, "    %s\n", c.subcmds[k].Info().Desc())
	}
}

// Info will return an Info describing either the currently selected subcommand, or
// the main "juju" tool if no subcommand has been chosen.
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
		func() {
			c.flag().PrintDefaults()
			c.DescCommands(os.Stderr)
		}}
}

// Parse will parse a complete command line. After normal option parsing is
// finished, the next arg will be used to look up the requested subcommand by
// name, and its Parse method will be called with all other remaining args.
func (c *JujuCommand) Parse(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no args to parse")
	}
	// Note: no arg interspersing, lest we deliver options to the wrong FlagSet
	if err := c.flag().Parse(false, args[1:]); err != nil {
		return err
	}
	args = c.flag().Args()
	if len(args) == 0 {
		return fmt.Errorf("no command specified")
	}
	exists := false
	if c.subcmd, exists = c.subcmds[args[0]]; !exists {
		return fmt.Errorf("unrecognised command: %s", args[0])
	}
	return c.subcmd.Parse(args[1:])
}

// Run will execute the subcommand selected in a previous call to Parse.
func (c *JujuCommand) Run() error {
	if c.subcmd == nil {
		return fmt.Errorf("no command selected")
	}
	return c.subcmd.Run()
}
