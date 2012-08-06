package main

import (
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
)

// AddUnitCommand is responsible adding a set of units to a service of the environment.
type AddUnitCommand struct {
	EnvName     string
	Num	int
}

func (c *AddUnitCommand) Info() *cmd.Info {
	return &cmd.Info{"add-unit", "", "add a service unit", ""}
}

func (c *AddUnitCommand) Init(f *gnuflag.FlagSet, args []string) error {
	addEnvironFlags(&c.EnvName, f)
	f.IntVar(&c.Num, "num-units", 1, "Number of service units to add.")
	if err := f.Parse(true, args); err != nil {
		return err
	}
	return cmd.CheckEmpty(f.Args())
}

// Run connects to the environment specified on the command line and calls 
// service.AddUnit the specified number of times.
func (c *AddUnitCommand) Run(_ *cmd.Context) error {
	conn, err := juju.NewConn(c.EnvName)
	if err != nil {
		return err
	}
	_ = conn
	return nil
}
