package control

import "flag"
import "fmt"

type BootstrapCommand struct {
	environment string
}

var _ Command = (*BootstrapCommand)(nil)

func (c *BootstrapCommand) Parse(args []string) error {
	fs := flag.NewFlagSet("bootstrap", flag.ExitOnError)
	fs.StringVar(&c.environment, "e", "", "juju environment to operate in")
	fs.StringVar(&c.environment, "environment", "", "juju environment to operate in")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(fs.Args()) != 0 {
		return fmt.Errorf("Unknown args: %s", fs.Args())
	}
	return nil
}

func (c *BootstrapCommand) Run() error {
	fmt.Println("Running bootstrap in environment ", c.environment)
	return nil
}
