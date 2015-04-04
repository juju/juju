// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space

import (
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/utils/set"
)

// CreateCommand calls the API to create a new network space.
type CreateCommand struct {
	SpaceCommandBase
	Name  string
	CIDRs set.Strings
}

const createCommandDoc = `
Creates a new space with the given name and associates the given list
of existing subnet CIDRs with it. At least one CIDR must be specified,
as except for the "default" space all other spaces must contain subnets.
`

// Info is defined on the cmd.Command interface.
func (c *CreateCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "create",
		Args:    "<name> [<CIDR1> <CIDR2> ...]",
		Purpose: "create a new network space",
		Doc:     strings.TrimSpace(createCommandDoc),
	}
}

// Init is defined on the cmd.Command interface. It checks the
// arguments for sanity and sets up the command to run.
func (c *CreateCommand) Init(args []string) error {
	name, CIDRs, err := ParseNameAndCIDRs(args)
	if err == nil {
		c.Name, c.CIDRs = name, CIDRs
	}
	return err
}

// Run implements Command.Run.
func (c *CreateCommand) Run(ctx *cmd.Context) error {
	return c.RunWithAPI(ctx, func(api SpaceAPI, ctx *cmd.Context) error {
		// Create the new space.
		err := api.CreateSpace(c.Name, c.CIDRs.SortedValues())
		if err != nil {
			return errors.Annotatef(err, "cannot create space %q", c.Name)
		}

		subnets_string := strings.Join(c.CIDRs.SortedValues(), ", ")
		ctx.Infof("created space %q with subnets %s", c.Name, subnets_string)

		return nil
	})
}
