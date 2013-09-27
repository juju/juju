// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"

	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/statecmd"
)

// GetConstraintsCommand shows the constraints for a service or environment.
type GetConstraintsCommand struct {
	cmd.EnvCommandBase
	ServiceName string
	out         cmd.Output
}

func (c *GetConstraintsCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "get-constraints",
		Args:    "[<service>]",
		Purpose: "view constraints",
	}
}

func formatConstraints(value interface{}) ([]byte, error) {
	return []byte(value.(constraints.Value).String()), nil
}

func (c *GetConstraintsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.EnvCommandBase.SetFlags(f)
	c.out.AddFlags(f, "constraints", map[string]cmd.Formatter{
		"constraints": formatConstraints,
		// TODO(nate): CONSTRAINTS_YAML: re-add yaml as a format when 
		// we can properly handle yaml serialization
		// see constraints/constrains.go
		// "yaml":        cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
}

func (c *GetConstraintsCommand) Init(args []string) error {
	if len(args) > 0 {
		if !names.IsService(args[0]) {
			return fmt.Errorf("invalid service name %q", args[0])
		}
		c.ServiceName, args = args[0], args[1:]
	}
	return cmd.CheckEmpty(args)
}

func (c *GetConstraintsCommand) Run(ctx *cmd.Context) error {
	conn, err := juju.NewConnFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer conn.Close()

	var cons constraints.Value
	if c.ServiceName != "" {
		args := params.GetServiceConstraints{
			ServiceName: c.ServiceName,
		}
		var results params.GetServiceConstraintsResults
		results, err = statecmd.GetServiceConstraints(conn.State, args)
		cons = results.Constraints
	} else {
		cons, err = conn.State.EnvironConstraints()
	}
	if err != nil {
		return err
	}
	return c.out.Write(ctx, cons)
}

// SetConstraintsCommand shows the constraints for a service or environment.
type SetConstraintsCommand struct {
	cmd.EnvCommandBase
	ServiceName string
	Constraints constraints.Value
}

func (c *SetConstraintsCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "set-constraints",
		Args:    "[key=[value] ...]",
		Purpose: "replace constraints",
	}
}

func (c *SetConstraintsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.EnvCommandBase.SetFlags(f)
	f.StringVar(&c.ServiceName, "s", "", "set service constraints")
	f.StringVar(&c.ServiceName, "service", "", "")
}

func (c *SetConstraintsCommand) Init(args []string) (err error) {
	if c.ServiceName != "" && !names.IsService(c.ServiceName) {
		return fmt.Errorf("invalid service name %q", c.ServiceName)
	}
	c.Constraints, err = constraints.Parse(args...)
	return err
}

func (c *SetConstraintsCommand) Run(_ *cmd.Context) (err error) {
	conn, err := juju.NewConnFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer conn.Close()
	if c.ServiceName == "" {
		return conn.State.SetEnvironConstraints(c.Constraints)
	}
	params := params.SetServiceConstraints{
		ServiceName: c.ServiceName,
		Constraints: c.Constraints,
	}
	return statecmd.SetServiceConstraints(conn.State, params)
}
