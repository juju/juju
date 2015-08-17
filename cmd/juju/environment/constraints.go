// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environment

import (
	"github.com/juju/cmd"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/constraints"
)

const getConstraintsDoc = `
Shows a list of constraints that have been set on the environment
using juju environment set-constraints.  You can also view constraints
set for a specific service by using juju service get-constraints <service>.

Constraints set on a service are combined with environment constraints for
commands (such as juju deploy) that provision machines for services.  Where
environment and service constraints overlap, the service constraints take
precedence.

See Also:
   juju help constraints
   juju help environment set-constraints
   juju help deploy
   juju help machine add
   juju help add-unit
`

const setConstraintsDoc = `
Sets machine constraints on the environment, which are used as the default
constraints for all new machines provisioned in the environment (unless
overridden).  You can also set constraints on a specific service by using
juju service set-constraints.

Constraints set on a service are combined with environment constraints for
commands (such as juju deploy) that provision machines for services.  Where
environment and service constraints overlap, the service constraints take
precedence.

Example:

   juju environment set-constraints mem=8G                         (all new machines in the environment must have at least 8GB of RAM)

See Also:
   juju help constraints
   juju help environment get-constraints
   juju help deploy
   juju help machine add
   juju help add-unit
`

// EnvGetConstraintsCommand shows the constraints for an environment.
// It is just a wrapper for the common GetConstraintsCommand and
// enforces that no service arguments are passed in.
type EnvGetConstraintsCommand struct {
	common.GetConstraintsCommand
}

func (c *EnvGetConstraintsCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "get-constraints",
		Purpose: "view constraints on the environment",
		Doc:     getConstraintsDoc,
	}
}

func (c *EnvGetConstraintsCommand) Init(args []string) error {
	return cmd.CheckEmpty(args)
}

// EnvSetConstraintsCommand sets the constraints for an environment.
// It is just a wrapper for the common SetConstraintsCommand and
// enforces that no service arguments are passed in.
type EnvSetConstraintsCommand struct {
	common.SetConstraintsCommand
}

func (c *EnvSetConstraintsCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "set-constraints",
		Args:    "[key=[value] ...]",
		Purpose: "set constraints on the environment",
		Doc:     setConstraintsDoc,
	}
}

// SetFlags overrides SetFlags for SetConstraintsCommand since that
// will register a flag to specify the service.
func (c *EnvSetConstraintsCommand) SetFlags(f *gnuflag.FlagSet) {}

func (c *EnvSetConstraintsCommand) Init(args []string) (err error) {
	c.Constraints, err = constraints.Parse(args...)
	return err
}
