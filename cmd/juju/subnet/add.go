// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnet

import (
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
)

// AddCommand calls the API to add an existing subnet to Juju.
type AddCommand struct {
	SubnetCommandBase

	CIDR  string
	Space names.SpaceTag
}

const addCommandDoc = `
Adds an existing subnet to Juju, making it available for use. Unlike
"juju subnet create", this command does not create a new subnet, so it
is supported on a wider variety of clouds (where SDN features are not
available, e.g. MAAS). The subnet will be associated with the given
existing Juju network space.

Any availablility zones associated with the added subnet are automatically
discovered using the cloud API (if supported).
`

// Info is defined on the cmd.Command interface.
func (c *AddCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "add",
		Args:    "<CIDR> <space>",
		Purpose: "add an existing subnet to Juju",
		Doc:     strings.TrimSpace(addCommandDoc),
	}
}

// Init is defined on the cmd.Command interface. It checks the
// arguments for sanity and sets up the command to run.
func (c *AddCommand) Init(args []string) error {
	// Ensure we have exactly 2 arguments.
	err := c.CheckNumArgs(args, []error{errNoCIDR, errNoSpace})
	if err != nil {
		return err
	}

	// Validate given CIDR.
	c.CIDR, err = c.ValidateCIDR(args[0])
	if err != nil {
		return err
	}

	// Validate the space name.
	c.Space, err = c.ValidateSpace(args[1])
	if err != nil {
		return err
	}

	return cmd.CheckEmpty(args[2:])
}

// Run implements Command.Run.
func (c *AddCommand) Run(ctx *cmd.Context) error {
	api, err := c.NewAPI()
	if err != nil {
		return errors.Annotate(err, "cannot connect to API server")
	}
	defer api.Close()

	// Add the existing subnet.
	err = api.AddSubnet(c.CIDR, c.Space)
	if err != nil {
		return errors.Annotatef(err, "cannot add subnet %q", c.CIDR)
	}

	ctx.Infof("added subnet %q in space %q", c.CIDR, c.Space.Id())
	return nil
}
