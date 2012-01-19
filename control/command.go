package control

import "flag"
import "fmt"
import "launchpad.net/juju/go/log"

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

func (c *JujuCommand) Logfile() string {
	return c.logfile
}

func (c *JujuCommand) Verbose() bool {
	return c.verbose
}

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

func (c *JujuCommand) Parse(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no args to parse")
	}
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	fs.StringVar(&c.logfile, "l", "", "where to log to")
	fs.StringVar(&c.logfile, "log-file", "", "where to log to")
	fs.BoolVar(&c.verbose, "v", false, "whether to be noisy")
	fs.BoolVar(&c.verbose, "verbose", false, "whether to be noisy")
	if err := fs.Parse(args[1:]); err != nil {
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

func (c *JujuCommand) Run() error {
	if c.subcmd == nil {
		return fmt.Errorf("no subcommand selected")
	}
	return c.subcmd.Run()
}

func JujuMain(args []string) error {
	jc := new(JujuCommand)
	jc.Register("bootstrap", new(BootstrapCommand))
	if err := jc.Parse(args); err != nil {
		return err
	}

	log.Debug = jc.Verbose()
	if err := log.SetFile(jc.Logfile()); err != nil {
		return err
	}
	return jc.Run()
}
