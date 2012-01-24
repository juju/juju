package main

import "fmt"
import "launchpad.net/~rogpeppe/juju/gnuflag/flag"
import "os"

// Command should "be implemented by" any subcommand that wants to be dispatched
// to by a JujuCommand.
type Command interface {
	PrintUsage()
	Parse(args []string) error
	Run() error
}

// JujuCommand handles top-level argument parsing and dispatch to subcommands.
type JujuCommand struct {
	flag    *flag.FlagSet
	logfile string
	verbose bool
	subcmds map[string]Command
	subcmd  Command
}

// NewJujuCommand will return a JujuCommand with the FlagSet set up, but no
// subcommands registered.
func NewJujuCommand() *JujuCommand {
	jc := &JujuCommand{}
	jc.subcmds = make(map[string]Command)
	jc.flag = flag.NewFlagSet("juju", flag.ExitOnError)
	jc.flag.StringVar(&jc.logfile, "l", "", "path to write log to")
	jc.flag.StringVar(&jc.logfile, "log-file", "", "path to write log to")
	jc.flag.BoolVar(&jc.verbose, "v", false, "if set, log additional messages")
	jc.flag.BoolVar(&jc.verbose, "verbose", false, "if set, log additional messages")
	jc.flag.Usage = func() { jc.PrintUsage() }
	return jc
}

// Logfile will return the logfile path specified on the command line, or "".
func (c *JujuCommand) Logfile() string {
	return c.logfile
}

// Verbose will return true if verbose was specified on the command line.
func (c *JujuCommand) Verbose() bool {
	return c.verbose
}

// Register will register a subcommand by name (which must not match that of a
// previously-registered subcommand), such that subsequent calls to Parse() will
// dispatch args following "name" to that subcommand for Parse()ing; and
// subsequent calls to Run() will call the subcommand's Run().
func (c *JujuCommand) Register(name string, subcmd Command) error {
	_, alreadythere := c.subcmds[name]
	if alreadythere {
		return fmt.Errorf("subcommand %s is already registered", name)
	}
	c.subcmds[name] = subcmd
	return nil
}

// PrintUsage will dump usage instructions to os.Stderr
func (c *JujuCommand) PrintUsage() {
	if c.subcmd != nil {
		c.subcmd.PrintUsage()
		return
	}
	fmt.Fprintln(os.Stderr, "usage: juju [options] <command> ...")
	c.flag.PrintDefaults()
	fmt.Fprintln(os.Stderr, "commands:")
}

// Parse will parse a complete command line. After normal option parsing is
// finished, the next arg will be used to look up the requested subcommand by
// name, and its Parse method will be called with all other remaining args.
func (c *JujuCommand) Parse(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no args to parse")
	}
	// Note: no arg interspersing, lest we deliver options to the wrong FlagSet
	if err := c.flag.Parse(false, args[1:]); err != nil {
		return err
	}
	args = c.flag.Args()
	if len(args) == 0 {
		return fmt.Errorf("no subcommand specified")
	}
	exists := false
	if c.subcmd, exists = c.subcmds[args[0]]; !exists {
		return fmt.Errorf("no %s subcommand registered", args[0])
	}
	return c.subcmd.Parse(args[1:])
}

// Run will execute the subcommand specified in a previous call to Parse.
func (c *JujuCommand) Run() error {
	if c.subcmd == nil {
		return fmt.Errorf("no subcommand selected")
	}
	return c.subcmd.Run()
}
