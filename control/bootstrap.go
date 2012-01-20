package control

import "fmt"
import "launchpad.net/~rogpeppe/juju/gnuflag/flag"

type BootstrapCommand struct {
	environment string
}

var _ Command = (*BootstrapCommand)(nil)

func (c *BootstrapCommand) Environment() string {
	return c.environment
}

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

func (c *BootstrapCommand) Run() error {
	return nil
}
