package main

import (
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
	//"launchpad.net/juju-core/state"
)

type StatusCommand struct {
	EnvName string
	conn    *juju.Conn
	out     cmd.Output
}

var statusDoc = "This command will report on the runtime state of various system entities."

func (c *StatusCommand) Info() *cmd.Info {
	return &cmd.Info{
		"status", "", "Output status information about an environment.", statusDoc,
	}
}

func (c *StatusCommand) Init(f *gnuflag.FlagSet, args []string) error {
	addEnvironFlags(&c.EnvName, f)
	c.out.AddFlags(f, "yaml", cmd.DefaultFormatters)
	if err := f.Parse(true, args); err != nil {
		return err
	}
	return cmd.CheckEmpty(f.Args())
}

func (c *StatusCommand) Run(ctx *cmd.Context) error {
	var err error
	c.conn, err = juju.NewConn(c.EnvName)
	if err != nil {
		return err
	}
	defer c.conn.Close()

	result := struct {
		Machines map[int]interface{}
		Services map[string]interface{}
	}{}

	result.Machines, err = c.processMachines()
	if err != nil {
		return err
	}

	result.Services, err = c.processServices()
	if err != nil {
		return err
	}

	return c.out.Write(ctx, result)
}

func (c *StatusCommand) processMachines() (map[int]interface{}, error) {
	machines := make(map[int]interface{})

	// TODO(dfc) process machines

	return machines, nil
}

func (c *StatusCommand) processServices() (map[string]interface{}, error) {
	services := make(map[string]interface{})

	// TODO(dfc) process services

	return services, nil
}
