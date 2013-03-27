package main

import (
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/statecmd"
)

// AddRelationCommand adds relations between service endpoints.
type AddRelationCommand struct {
	EnvCommandBase
	Endpoints []string
	out       cmd.Output
}

func (c *AddRelationCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "add-relation",
		Args:    "<service1>[:<relation name1>] <service2>[:<relation name2>]",
		Purpose: "add a relation between two services",
	}
}

func (c *AddRelationCommand) Init(args []string) error {
	c.Endpoints = args
	return nil
}

// Run adds a relation and and formats the result as a YAML string.
func (c *AddRelationCommand) Run(ctx *cmd.Context) error {
	conn, err := juju.NewConnFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer conn.Close()
	params := params.AddRelation{
		Endpoints: c.Endpoints,
	}
	relInfo, err := statecmd.AddRelation(conn.State, params)
	if err != nil {
		return err
	}
	relInfoMap := map[string]interface{}{
		"endpoints": relInfo.Endpoints,
		"interface": relInfo.Interface,
		"scope":     relInfo.Scope,
	}
	return c.out.Write(ctx, relInfoMap)
}
