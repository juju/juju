package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
)

// AddRelationCommand adds relations between service endpoints.
type AddRelationCommand struct {
	EnvName   string
	Endpoints []string
}

func (c *AddRelationCommand) Info() *cmd.Info {
	return &cmd.Info{"add-relation", "<service>[:<relation>][ ...]", "add a service relation", ""}
}

func (c *AddRelationCommand) Init(f *gnuflag.FlagSet, args []string) error {
	addEnvironFlags(&c.EnvName, f)
	if err := f.Parse(true, args); err != nil {
		return err
	}
	args = f.Args()
	switch len(args) {
	case 1, 2:
		c.Endpoints = args
	default:
		return fmt.Errorf("a relation must involve one or two services")
	}
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
