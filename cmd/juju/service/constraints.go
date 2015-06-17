// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/names"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/constraints"
)

const getConstraintsDoc = `
Shows the list of constraints that have been set on the specified service
using juju service set-constraints.  You can also view constraints
set for an environment by using juju environment get-constraints.

Constraints set on a service are combined with environment constraints for
commands (such as juju deploy) that provision machines for services.  Where
environment and service constraints overlap, the service constraints take
precedence.

See Also:
   juju help constraints
   juju help service set-constraints
   juju help deploy
   juju help machine add
   juju help add-unit
`

const setConstraintsDoc = `
Sets machine constraints on specific service, which are used as the
default constraints for all new machines provisioned by that service.
You can also set constraints on an environment by using
juju environment set-constraints.

Constraints set on a service are combined with environment constraints for
commands (such as juju deploy) that provision machines for services.  Where
environment and service constraints overlap, the service constraints take
precedence.

Example:

    set-constraints wordpress mem=4G     (all new wordpress machines must have at least 4GB of RAM)

See Also:
   juju help constraints
   juju help service get-constraints
   juju help deploy
   juju help machine add
   juju help add-unit
`

// ServiceGetConstraintsCommand shows the constraints for a service.
// It is just a wrapper for the common GetConstraintsCommand which
// enforces that a service is specified.
type ServiceGetConstraintsCommand struct {
	common.GetConstraintsCommand
}

func (c *ServiceGetConstraintsCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "get-constraints",
		Args:    "<service>",
		Purpose: "view constraints on a service",
		Doc:     getConstraintsDoc,
	}
}

func (c *ServiceGetConstraintsCommand) Init(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no service name specified")
	}
	if !names.IsValidService(args[0]) {
		return fmt.Errorf("invalid service name %q", args[0])
	}

	c.ServiceName = args[0]
	return nil
}

// ServiceSetConstraintsCommand sets the constraints for a service.
// It is just a wrapper for the common SetConstraintsCommand which
// enforces that a service is specified.
type ServiceSetConstraintsCommand struct {
	common.SetConstraintsCommand
}

func (c *ServiceSetConstraintsCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "set-constraints",
		Args:    "<service> [key=[value] ...]",
		Purpose: "set constraints on a service",
		Doc:     setConstraintsDoc,
	}
}

// SetFlags overrides SetFlags for SetConstraintsCommand since that
// will register a flag to specify the service, and the flag is not
// required with this service supercommand.
func (c *ServiceSetConstraintsCommand) SetFlags(f *gnuflag.FlagSet) {}

func (c *ServiceSetConstraintsCommand) Init(args []string) (err error) {
	if len(args) == 0 {
		return fmt.Errorf("no service name specified")
	}
	if !names.IsValidService(args[0]) {
		return fmt.Errorf("invalid service name %q", args[0])
	}

	c.ServiceName, args = args[0], args[1:]

	c.Constraints, err = constraints.Parse(args...)
	return err
}
