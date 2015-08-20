// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnet

import (
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
)

// RemoveCommand calls the API to remove an existing, unused subnet
// from Juju.
type RemoveCommand struct {
	SubnetCommandBase

	CIDR names.SubnetTag
}

const removeCommandDoc = `
Marks an existing subnet for removal. Depending on what features the
cloud infrastructure supports, this command will either delete the
subnet using the cloud API (if supported, e.g. in Amazon VPC) or just
remove the subnet entity from Juju's database (with non-SDN substrates,
e.g. MAAS). In other words "remove" acts like the opposite of "create"
(if supported) or "add" (if "create" is not supported).

If any machines are still using the subnet, it cannot be removed and
an error is returned instead. If the subnet is not in use, it will be
marked for removal, but it will not be removed from the Juju database
until all related entites are cleaned up (e.g. allocated addresses).
`

// Info is defined on the cmd.Command interface.
func (c *RemoveCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "remove",
		Args:    "<CIDR>",
		Purpose: "remove an existing subnet from Juju",
		Doc:     strings.TrimSpace(removeCommandDoc),
	}
}

// Init is defined on the cmd.Command interface. It checks the
// arguments for sanity and sets up the command to run.
func (c *RemoveCommand) Init(args []string) error {
	// Ensure we have exactly 1 argument.
	err := c.CheckNumArgs(args, []error{errNoCIDR})
	if err != nil {
		return err
	}

	// Validate given CIDR.
	c.CIDR, err = c.ValidateCIDR(args[0], true)
	if err != nil {
		return err
	}

	return cmd.CheckEmpty(args[1:])
}

// Run implements Command.Run.
func (c *RemoveCommand) Run(ctx *cmd.Context) error {
	return c.RunWithAPI(ctx, func(api SubnetAPI, ctx *cmd.Context) error {
		// Try removing the subnet.
		if err := api.RemoveSubnet(c.CIDR); err != nil {
			return errors.Annotatef(err, "cannot remove subnet %q", c.CIDR.Id())
		}

		ctx.Infof("marked subnet %q for removal", c.CIDR.Id())
		return nil
	})
}
