package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
)

// DestroyRelationCommand causes an existing service relation to be shut down.
type DestroyRelationCommand struct {
	EnvName   string
	Endpoints []string
}

func (c *DestroyRelationCommand) Info() *cmd.Info {
	return &cmd.Info{
		"destroy-relation", "<service1>[:<relation name1>] <service2>[:<relation name2>]",
		"destroy a relation between two services", "",
	}
}

func (c *DestroyRelationCommand) SetFlags(f *gnuflag.FlagSet) {
	addEnvironFlags(&c.EnvName, f)
}

func (c *DestroyRelationCommand) Init(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("a relation must involve two services")
	}
	c.Endpoints = args
	return nil
}

func (c *DestroyRelationCommand) Run(_ *cmd.Context) error {
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
