package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
)

// RemoveRelationCommand causes an existing serive relation to be shut down.
type RemoveRelationCommand struct {
	EnvName   string
	Endpoints []string
}

func (c *RemoveRelationCommand) Info() *cmd.Info {
	return &cmd.Info{
		"remove-relation", "<service1>[:<relation name1>] <service2>[:<relation name2>]",
		"remove a relation between two services", "",
	}
}

func (c *RemoveRelationCommand) Init(f *gnuflag.FlagSet, args []string) error {
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

func (c *RemoveRelationCommand) Run(_ *cmd.Context) error {
	conn, err := juju.NewConnFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer conn.Close()
	eps, err := conn.State.InferEndpoints(c.Endpoints)
	if err != nil {
		return err
	}
	rel, err := conn.State.EndpointsRelation(eps...)
	if err != nil {
		return err
	}
	return rel.Destroy()
}
