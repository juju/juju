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

   set-constraints mem=8G                         (all new machines in the environment must have at least 8GB of RAM)
   set-constraints --service wordpress mem=4G     (all new wordpress machines can ignore the 8G constraint above, and require only 4G)

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

// getEnvironConstraints1dot16 uses direct DB access to get the Environment
// constraints against an API server running 1.16 or older (when GetEnvironmentConstraints
// was not available). This fallback can be removed when we no longer maintain
// 1.16 compatibility.
// This only does the GetEnvironmentConstraints portion of Run, since
// GetServiceConstraints was already implemented.
func (c *GetConstraintsCommand) getEnvironConstraints1dot16() (constraints.Value, error) {
	conn, err := juju.NewConnFromName(c.EnvName)
	if err != nil {
		return constraints.Value{}, err
	}
	defer conn.Close()
	return conn.State.EnvironConstraints()
}

func (c *GetConstraintsCommand) Run(ctx *cmd.Context) error {
	apiclient, err := juju.NewAPIClientFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer apiclient.Close()

	var cons constraints.Value
	if c.ServiceName == "" {
		cons, err = apiclient.GetEnvironmentConstraints()
		if params.IsCodeNotImplemented(err) {
			logger.Infof("GetEnvironmentConstraints not supported by the API server, " +
				"falling back to 1.16 compatibility mode (direct DB access)")
			cons, err = c.getEnvironConstraints1dot16()
		}
	} else {
		cons, err = apiclient.GetServiceConstraints(c.ServiceName)
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

// setEnvironConstraints1dot16 uses direct DB access to get the Environment
// constraints against an API server running 1.16 or older (when SetEnvironmentConstraints
// was not available). This fallback can be removed when we no longer maintain
// 1.16 compatibility.
// This only does the SetEnvironmentConstraints portion of Run, since
// SetServiceConstraints was already implemented.
func (c *SetConstraintsCommand) setEnvironConstraints1dot16() error {
	conn, err := juju.NewConnFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.State.SetEnvironConstraints(c.Constraints)
}

func (c *SetConstraintsCommand) Run(_ *cmd.Context) (err error) {
	apiclient, err := juju.NewAPIClientFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer apiclient.Close()
	if c.ServiceName == "" {
		err = apiclient.SetEnvironmentConstraints(c.Constraints)
		if params.IsCodeNotImplemented(err) {
			logger.Infof("SetEnvironmentConstraints not supported by the API server, " +
				"falling back to 1.16 compatibility mode (direct DB access)")
			err = c.setEnvironConstraints1dot16()
		}
		return err
	}
	return apiclient.SetServiceConstraints(c.ServiceName, c.Constraints)
}
