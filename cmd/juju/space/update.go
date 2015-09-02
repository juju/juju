// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space

import (
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/utils/set"
	"launchpad.net/gnuflag"
)

// UpdateCommand calls the API to update an existing network space.
type UpdateCommand struct {
	SpaceCommandBase
	Name  string
	CIDRs set.Strings
}

const updateCommandDoc = `
Replaces the list of associated subnets of the space. Since subnets
can only be part of a single space, all specified subnets (using their
CIDRs) "leave" their current space and "enter" the one we're updating.
`

func (c *UpdateCommand) SetFlags(f *gnuflag.FlagSet) {
	c.SpaceCommandBase.SetFlags(f)
}

// Info is defined on the cmd.Command interface.
func (c *UpdateCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "update",
		Args:    "<name> <CIDR1> [ <CIDR2> ...]",
		Purpose: "update a network space's CIDRs",
		Doc:     strings.TrimSpace(updateCommandDoc),
	}
}

// Init is defined on the cmd.Command interface. It checks the
// arguments for sanity and sets up the command to run.
func (c *UpdateCommand) Init(args []string) error {
	var err error
	c.Name, c.CIDRs, err = ParseNameAndCIDRs(args, false)
	return errors.Trace(err)
}

// Run implements Command.Run.
func (c *UpdateCommand) Run(ctx *cmd.Context) error {
	return c.RunWithAPI(ctx, func(api SpaceAPI, ctx *cmd.Context) error {
		// Update the space.
		err := api.UpdateSpace(c.Name, c.CIDRs.SortedValues())
		if err != nil {
			return errors.Annotatef(err, "cannot update space %q", c.Name)
		}

		ctx.Infof("updated space %q: changed subnets to %s", c.Name, strings.Join(c.CIDRs.SortedValues(), ", "))
		return nil
	})
}
