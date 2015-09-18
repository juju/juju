// Copyright 2013 - 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/names"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/constraints"
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

// NewGetConstraintsCommand return a new command that returns a
// list of constraints that have been set on the environment.
func NewGetConstraintsCommand() cmd.Command {
	return envcmd.Wrap(&GetConstraintsCommand{})
}

// GetConstraintsCommand shows the constraints for a service or environment.
type GetConstraintsCommand struct {
	envcmd.EnvCommandBase
	ServiceName string
	out         cmd.Output
	api         ConstraintsAPI
}

// Constraints API defines methods on the client API that
// the get-constraints and set-constraints commands call
type ConstraintsAPI interface {
	Close() error
	GetEnvironmentConstraints() (constraints.Value, error)
	GetServiceConstraints(string) (constraints.Value, error)
	SetEnvironmentConstraints(constraints.Value) error
	SetServiceConstraints(string, constraints.Value) error
}

func (c *GetConstraintsCommand) getAPI() (ConstraintsAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	return c.NewAPIClient()
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
	c.out.AddFlags(f, "constraints", map[string]cmd.Formatter{
		"constraints": formatConstraints,
		"yaml":        cmd.FormatYaml,
		"json":        cmd.FormatJson,
	})
}

func (c *GetConstraintsCommand) Init(args []string) error {
	if len(args) > 0 {
		if !names.IsValidService(args[0]) {
			return fmt.Errorf("invalid service name %q", args[0])
		}
		c.ServiceName, args = args[0], args[1:]
	}
	return cmd.CheckEmpty(args)
}

func (c *GetConstraintsCommand) Run(ctx *cmd.Context) error {
	apiclient, err := c.getAPI()
	if err != nil {
		return err
	}
	defer apiclient.Close()

	var cons constraints.Value
	if c.ServiceName == "" {
		cons, err = apiclient.GetEnvironmentConstraints()
	} else {
		cons, err = apiclient.GetServiceConstraints(c.ServiceName)
	}
	if err != nil {
		return err
	}
	return c.out.Write(ctx, cons)
}

// NewSetConstraintsCommands returns a new command that sets machine
// constraints on the system, which are used as the default
// constraints for all new machines provisioned in the environment
// (unless overridden).
func NewSetConstraintsCommand() cmd.Command {
	return envcmd.Wrap(&SetConstraintsCommand{})
}

// SetConstraintsCommand shows the constraints for a service or environment.
type SetConstraintsCommand struct {
	envcmd.EnvCommandBase
	ServiceName string
	api         ConstraintsAPI
	Constraints constraints.Value
}

func (c *SetConstraintsCommand) getAPI() (ConstraintsAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	return c.NewAPIClient()
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
	f.StringVar(&c.ServiceName, "s", "", "set service constraints")
	f.StringVar(&c.ServiceName, "service", "", "")
}

func (c *SetConstraintsCommand) Init(args []string) (err error) {
	if c.ServiceName != "" && !names.IsValidService(c.ServiceName) {
		return fmt.Errorf("invalid service name %q", c.ServiceName)
	}
	c.Constraints, err = constraints.Parse(args...)
	return err
}

func (c *SetConstraintsCommand) Run(_ *cmd.Context) (err error) {
	apiclient, err := c.getAPI()
	if err != nil {
		return err
	}
	defer apiclient.Close()

	if c.ServiceName == "" {
		err = apiclient.SetEnvironmentConstraints(c.Constraints)
	} else {
		err = apiclient.SetServiceConstraints(c.ServiceName, c.Constraints)
	}
	return block.ProcessBlockedError(err, block.BlockChange)
}
