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

	CIDR       string
	RawCIDR    string // before normalizing (e.g. 10.10.0.0/8 to 10.0.0.0/8)
	ProviderId string
	Space      names.SpaceTag
	Zones      []string
}

const addCommandDoc = `
Adds an existing subnet to Juju, making it available for use. Unlike
"juju subnet create", this command does not create a new subnet, so it
is supported on a wider variety of clouds (where SDN features are not
available, e.g. MAAS). The subnet will be associated with the given
existing Juju network space.

Subnets can be referenced by either their CIDR or provider-specific ID.
If CIDR is used an multiple subnets have the same CIDR, an error will
be returned, including the list of possible provider IDs uniquely
identifying each subnet.

Any availablility zones associated with the added subnet are automatically
discovered using the cloud API (if supported). If this is not possible,
since any subnet needs to be part of at least one zone, specifying
zone(s) is required.
`

// Info is defined on the cmd.Command interface.
func (c *AddCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "add",
		Args:    "<CIDR>|<provider-id> <space> [<zone1> <zone2> ...]",
		Purpose: "add an existing subnet to Juju",
		Doc:     strings.TrimSpace(addCommandDoc),
	}
}

// Init is defined on the cmd.Command interface. It checks the
// arguments for sanity and sets up the command to run.
func (c *AddCommand) Init(args []string) error {
	// Ensure we have 2 or more arguments.
	switch len(args) {
	case 0:
		return errNoCIDROrID
	case 1:
		return errNoSpace
	}

	// Try to validate first argument as a CIDR first.
	var err error
	c.RawCIDR = args[0]
	c.CIDR, err = c.ValidateCIDR(args[0], false)
	if err != nil {
		// If it's not a CIDR it could be a ProviderId, so ignore the
		// error.
		c.ProviderId = args[0]
		c.RawCIDR = ""
	}

	// Validate the space name.
	c.Space, err = c.ValidateSpace(args[1])
	if err != nil {
		return err
	}

	// Add any given zones.
	for _, zone := range args[2:] {
		c.Zones = append(c.Zones, zone)
	}
	return nil
}

// Run implements Command.Run.
func (c *AddCommand) Run(ctx *cmd.Context) error {
	api, err := c.NewAPI()
	if err != nil {
		return errors.Annotate(err, "cannot connect to API server")
	}
	defer api.Close()

	if c.CIDR != "" && c.RawCIDR != c.CIDR {
		ctx.Infof(
			"WARNING: using CIDR %q instead of the incorrectly specified %q.",
			c.CIDR, c.RawCIDR,
		)
	}

	// Add the existing subnet.
	err = api.AddSubnet(c.CIDR, c.ProviderId, c.Space, c.Zones)
	// TODO(dimitern): Change this once the API returns a concrete error.
	if err != nil && strings.Contains(err.Error(), "multiple subnets with") {
		// Special case: multiple subnets with the same CIDR exist
		ctx.Infof("ERROR: %v.", err)
		return nil
	} else if err != nil {
		return errors.Annotatef(err, "cannot add subnet %q", c.CIDR)
	}

	if c.CIDR == "" {
		ctx.Infof("added subnet with ProviderId %q in space %q", c.ProviderId, c.Space.Id())
	} else {
		ctx.Infof("added subnet with CIDR %q in space %q", c.CIDR, c.Space.Id())
	}
	return nil
}
