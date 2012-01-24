package main

import "fmt"
import "launchpad.net/juju/go/juju"
import "launchpad.net/~rogpeppe/juju/gnuflag/flag"
import "os"

// BootstrapCommand is responsible for launching the first machine in a juju
// environment, and setting up everything necessary to continue working.
type BootstrapCommand struct {
	flag        *flag.FlagSet
	environment string
}

// Ensure Command interface
var _ Command = (*BootstrapCommand)(nil)

func NewBootstrapCommand() *BootstrapCommand {
	c := &BootstrapCommand{}
	c.flag = flag.NewFlagSet("bootstrap", flag.ExitOnError)
	c.flag.StringVar(&c.environment, "e", "", "juju environment to operate in")
	c.flag.StringVar(&c.environment, "environment", "", "juju environment to operate in")
	c.flag.Usage = func() { c.PrintUsage() }
	return c
}

// Environment returns the name of the environment to be bootstrapped.
func (c *BootstrapCommand) Environment() string {
	return c.environment
}

// Parse takes the list of args following "bootstrap" on the command line, and
// will initialise the BootstrapCommand such that it uses them when Run()ning.
func (c *BootstrapCommand) Parse(args []string) error {
	// Parse(true, ...) is meaningless is this specific case, but is generally
	// required for juju subcommands, because many of them *do* have positional
	// arguments, and we need to allow interspersion to match the Python version.
	if err := c.flag.Parse(true, args); err != nil {
		return err
	}
	if len(c.flag.Args()) != 0 {
		return fmt.Errorf("Unknown args: %s", c.flag.Args())
	}
	return nil
}

func (c *BootstrapCommand) PrintUsage() {
	fmt.Fprintln(os.Stderr, "usage: juju bootstrap [options]")
	c.flag.PrintDefaults()
}

// Run will bootstrap the juju environment set in Parse, or the default environment
// if none has been set.
func (c *BootstrapCommand) Run() error {
	conn, err := juju.NewConn(c.Environment())
	if err != nil {
		return err
	}
	return conn.Bootstrap()
}
