package control

import "fmt"
import "launchpad.net/juju/go/log"
import "launchpad.net/~rogpeppe/juju/gnuflag/flag"

// BootstrapCommand is responsible for launching the first machine in a juju
// environment, and setting up everything necessary to continue working.
type BootstrapCommand struct {
	environment string
}

var _ Command = (*BootstrapCommand)(nil)

// Environment returns the name of the environment to be bootstrapped.
func (c *BootstrapCommand) Environment() string {
	return c.environment
}

// Parse takes the list of args following "bootstrap" on the command line, and
// will initialise the BootstrapCommand such that it uses them when Run()ning.
func (c *BootstrapCommand) Parse(args []string) error {
	fs := flag.NewFlagSet("bootstrap", flag.ContinueOnError)
	fs.StringVar(&c.environment, "e", "", "juju environment to operate in")
	fs.StringVar(&c.environment, "environment", "", "juju environment to operate in")

	// normal flag usage output is not really appropriate
	fs.Usage = func() {}

	// ParseGnu(true, ...) is meaningless is this specific case, but is generally
	// required for juju subcommands, because many of them *do* have positional
	// arguments, and we need to allow interspersion to match the Python version.
	if err := fs.ParseGnu(true, args); err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("Unknown args: %s", fs.Args())
	}
	return nil
}

// Usage will return instructions for using the juju bootstrap command. It's
// currently somewhat unhelpful.
func (c *BootstrapCommand) Usage() string {
	return "You're Doing bootstrap Wrong."
}

// Run will bootstrap the juju environment set in Parse, or the default environment
// if none has been set.
func (c *BootstrapCommand) Run() error {
	log.Printf("Bootstrapping environment: %s\n", c.Environment())
	return nil
}
