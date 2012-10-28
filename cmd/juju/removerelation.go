package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/log"
)

// RemoveRelationCommand causes an existing serive relation to be shut down.
type RemoveRelationCommand struct {
	EnvName   string
	Endpoints []string
}

func (c *RemoveRelationCommand) Info() *cmd.Info {
	return &cmd.Info{"remove-relation", "<service>[:<relation>][ ...]", "remove a service relation", ""}
}

func (c *RemoveRelationCommand) Init(f *gnuflag.FlagSet, args []string) error {
	addEnvironFlags(&c.EnvName, f)
	if err := f.Parse(true, args); err != nil {
		return err
	}
	args = f.Args()
	log.Printf("%#v", args)
	switch len(args) {
	case 1, 2:
		c.Endpoints = args
	default:
		return fmt.Errorf("a relation must involve one or two services")
	}
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
