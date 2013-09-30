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

const getConstraintsDoc = `
get-constraints returns a list of constraints that have been set on
the environment using juju set-constraints.  You can also view constraints set
for a specific service by using juju get-constraints <service>.

See Also:
   juju help constraints
   juju help set-constraints
`

const setConstraintsDoc = `
set-constraints sets machine constraints on the system, which are used as the
default constraints for all new machines provisioned in the environment (unless
overridden).  You can also set constraints on a specific service by using juju 
set-constraints <service>. 

Constraints set on a service are combined with environment constraints for
commands (such as juju deploy) that provision machines for services.  Where
environment and service constraints overlap, the service constraints take
precedence.

Examples:

   set-constraints mem=8G               (all new machines in the environment must have at least 8GB of RAM)
   set-constraints mysql root-disk=99G  (all new machines provisioned for mysql must have at least 99GB of disk space)

See Also:
   juju help constraints
   juju help get-constraints
   juju help deploy
   juju help add-machine
   juju help add-unit
`

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
		Purpose: "view constraints on the environment or a service",
		Doc:     getConstraintsDoc,
	}
}

func formatConstraints(value interface{}) ([]byte, error) {
	return []byte(value.(constraints.Value).String()), nil
}

func (c *GetConstraintsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.EnvCommandBase.SetFlags(f)
	c.out.AddFlags(f, "constraints", map[string]cmd.Formatter{
		"constraints": formatConstraints,
		"yaml":        cmd.FormatYaml,
		"json":        cmd.FormatJson,
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
		Purpose: "set constraints on the environment or a service",
		Doc:     setConstraintsDoc,
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
