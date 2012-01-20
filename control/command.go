package control

import "flag"
import "fmt"

type Command interface {
	Parse(args []string) error
	Run() error
}

type JujuCommand struct {
	logfile string
	verbose bool
	subcmds map[string]Command
	subcmd  Command
}

// Path to logfile specified on command line, or "".
func (c *JujuCommand) Logfile() string {
	return c.logfile
}

// true if verbose was specified on command line.
func (c *JujuCommand) Verbose() bool {
	return c.verbose
}

// Register a subcommand by name, which must not match that of a previously-
// registered subcommand.
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

// Parse a command line. After normal option parsing is complete, the next arg
// will be used to look up the requested subcommand by name, and its Parse
// method will be called with all remaining args after the name.
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

	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	return c.parseSubcmd(fs.Args())
}

func (c *JujuCommand) Usage() {
	fmt.Println("You're Doing It Wrong.")
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

// Run the subcommand specified in a previous call to Parse.
func (c *JujuCommand) Run() error {
	if c.subcmd == nil {
		return fmt.Errorf("no subcommand selected")
	}
	return c.subcmd.Run()
}

var jujuMainCommands = map[string]Command{"bootstrap": new(BootstrapCommand)}

// Return a JujuCommand for the main "juju" executable.
func JujuMainCommand() *JujuCommand {
	jc := new(JujuCommand)
	for name, subcmd := range jujuMainCommands {
		jc.Register(name, subcmd)
	}
	return jc
}
