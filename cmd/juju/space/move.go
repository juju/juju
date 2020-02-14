// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space

import (
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/collections/set"
	"github.com/juju/errors"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
)

// NewMoveCommand returns a command used to update subnets in a space.
func NewMoveCommand() modelcmd.ModelCommand {
	return modelcmd.Wrap(&MoveCommand{})
}

// MoveCommand calls the API to update an existing network space.
type MoveCommand struct {
	SpaceCommandBase
	Name  string
	CIDRs set.Strings
}

const updateCommandDoc = `
Replaces the list of associated subnets of the space. Since subnets
can only be part of a single space, all specified subnets (using their
CIDRs) "leave" their current space and "enter" the one we're updating.
`

// Info is defined on the cmd.Command interface.
func (c *MoveCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "move-to-space",
		Args:    "<name> <CIDR1> [ <CIDR2> ...]",
		Purpose: "Update a network space's CIDRs.",
		Doc:     strings.TrimSpace(updateCommandDoc),
	})
}

// Init is defined on the cmd.Command interface. It checks the
// arguments for sanity and sets up the command to run.
func (c *MoveCommand) Init(args []string) error {
	var err error
	c.Name, c.CIDRs, err = ParseNameAndCIDRs(args, false)
	return errors.Trace(err)
}

// Run implements Command.Run.
func (c *MoveCommand) Run(ctx *cmd.Context) error {
	return c.RunWithAPI(ctx, func(api SpaceAPI, ctx *cmd.Context) error {
		// Update the space.
		err := api.MoveToSpace(c.Name, c.CIDRs.SortedValues())
		if err != nil {
			return errors.Annotatef(err, "cannot update space %q", c.Name)
		}

		//ctx.Infof("updated space %q: changed subnets to %s", c.Name, strings.Join(c.CIDRs.SortedValues(), ", "))
		return nil
	})
}
