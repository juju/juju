package control

import "fmt"
import "launchpad.net/~rogpeppe/juju/gnuflag/flag"

// Command should be implemented by any subcommand that wants to be dispatched
// to by a JujuCommand.
type Command interface {
	Parse(args []string) error
	Usage() string
	Run() error
}

// JujuCommand handles top-level argument parsing and dispatch to subcommands.
type JujuCommand struct {
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

// Register will register a subcommand by name (which must not match that of a
// previously-registered subcommand), such that subsequent calls to Parse() will
// dispatch args following "name" to that subcommand for Parse()ing; and
// subsequent calls to Run() will call the subcommand's Run().
func (c *JujuCommand) Register(name string, subcmd Command) error {
	if c.subcmds == nil {
		c.subcmds = make(map[string]Command)
	}
	_, alreadythere := c.subcmds[name]
	if alreadythere {
		return fmt.Errorf("subcommand %s is already registered", name)
	}
	c.subcmds[name] = subcmd
	return nil
}

// Usage will return instructions for using this JujuCommand or the selected
// subcommand. It isn't currently very helpful.
func (c *JujuCommand) Usage() string {
	if c.subcmd != nil {
		return c.subcmd.Usage()
	}
	return "You're Doing It Wrong."
}

// Parse will parse a complete command line. After normal option parsing is
// finished, the next arg will be used to look up the requested subcommand by
// name, and its Parse method will be called with all other remaining args.
func (c *JujuCommand) Parse(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no args to parse")
	}
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	fs.StringVar(&c.logfile, "l", "", "where to log to")
	fs.StringVar(&c.logfile, "log-file", "", "where to log to")
	fs.BoolVar(&c.verbose, "v", false, "whether to be noisy")
	fs.BoolVar(&c.verbose, "verbose", false, "whether to be noisy")

	// normal flag usage output is not really appropriate
	fs.Usage = func() {}

	// no arg interspersing, lest we deliver options to the wrong FlagSet
	if err := fs.ParseGnu(false, args[1:]); err != nil {
		return err
	}
	return c.parseSubcmd(fs.Args())
}

func (c *JujuCommand) parseSubcmd(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no subcommand specified")
	}
	if c.subcmds == nil {
		return fmt.Errorf("no subcommands registered")
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

var jujuMainCommands = map[string]Command{"bootstrap": new(BootstrapCommand)}

// JujuMainCommand will return a JujuCommand for the main "juju" executable.
func JujuMainCommand() *JujuCommand {
	jc := new(JujuCommand)
	for name, subcmd := range jujuMainCommands {
		jc.Register(name, subcmd)
	}
	return jc
}
