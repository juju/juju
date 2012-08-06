package main

import (
	"errors"

	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
)

// AddUnitCommand is responsible adding additional units to a service.
type AddUnitCommand struct {
	EnvName     string
	ServiceName string
	NumUnits    int
}

func (c *AddUnitCommand) Info() *cmd.Info {
	return &cmd.Info{"add-unit", "", "add a service unit", ""}
}

func (c *AddUnitCommand) Init(f *gnuflag.FlagSet, args []string) error {
	addEnvironFlags(&c.EnvName, f)
	f.IntVar(&c.NumUnits, "num-units", 1, "Number of service units to add.")
	if err := f.Parse(true, args); err != nil {
		return err
	}
	args = f.Args()
	switch len(args) {
	case 1:
		c.ServiceName = args[0]
	case 0:
		return errors.New("no service specified")
	default:
		return cmd.CheckEmpty(args[1:])
	}
	if c.NumUnits < 1 {
		return errors.New("must add at least one unit")
	}
	return nil
}

// Run connects to the environment specified on the command line 
// and calls conn.AddUnits.
func (c *AddUnitCommand) Run(_ *cmd.Context) error {
	conn, err := juju.NewConn(c.EnvName)
	if err != nil {
		return err
	}
	defer conn.Close()
	st, err := conn.State()
	if err != nil {
		return err
	}
	service, err := st.Service(c.ServiceName)
	if err != nil {
		return err
	}
	_, err = conn.AddUnits(service, c.NumUnits)
	return err

}
