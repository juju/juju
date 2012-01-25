package main

import (
	"fmt"
	"launchpad.net/~rogpeppe/juju/gnuflag/flag"
)

type TestCommand struct {
	Name  string
	Value string
}

func (c *TestCommand) Parse(args []string) error {
	fs := flag.NewFlagSet(c.Name, flag.ContinueOnError)
	fs.StringVar(&c.Value, "value", "", "doc")
	return fs.Parse(true, args)
}

func (c *TestCommand) Info() *Info {
	return &Info{
		c.Name,
		"",
		fmt.Sprintf("command named %s", c.Name),
		"",
		func() {}}
}

func (c *TestCommand) Run() error {
	return fmt.Errorf("BORKEN: value is %s.", c.Value)
}
