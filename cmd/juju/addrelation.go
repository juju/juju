package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
)

// AddRelationCommand adds relations between services.
type AddRelationCommand struct {
	EnvName   string
	Endpoints []string
}

func (c *AddRelationCommand) Info() *cmd.Info {
	return &cmd.Info{
		"add-relation", "<service1>[:<relation name1>] <service2>[:<relation name2>]",
		"add a relation between two services", "",
	}
}

func (c *AddRelationCommand) Init(f *gnuflag.FlagSet, args []string) error {
	addEnvironFlags(&c.EnvName, f)
	if err := f.Parse(true, args); err != nil {
		return err
	}
	args = f.Args()
	if len(args) != 2 {
		return fmt.Errorf("a relation must involve two services")
	}
	c.Endpoints = args
	return nil
}

func (c *AddRelationCommand) Run(_ *cmd.Context) error {
	conn, err := juju.NewConnFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer conn.Close()
	eps, err := conn.State.InferEndpoints(c.Endpoints)
	if err != nil {
		return err
	}
	_, err = conn.State.AddRelation(eps...)
	return err
}
