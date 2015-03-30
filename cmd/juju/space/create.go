// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space

import (
	"net"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils/set"
)

// CreateCommand calls the API to create a new network space.
type CreateCommand struct {
	SpaceCommandBase

	Name  string
	CIDRs set.Strings
}

const createCommandDoc = `
Creates a new network space with a given name, optionally including one or more
subnets specified with their CIDR values.

A network space name can consist of ...
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
	// Validate given name.
	if len(args) == 0 {
		return errors.New("space name is required")
	}
	givenName := args[0]
	if !names.IsValidSpace(givenName) {
		return errors.Errorf("%q is not a valid space name", givenName)
	}
	c.Name = givenName

	// Validate any given CIDRs.
	c.CIDRs = set.NewStrings()
	for _, arg := range args[1:] {
		_, ipNet, err := net.ParseCIDR(arg)
		if err != nil {
			logger.Debugf("cannot parse %q: %v", arg, err)
			return errors.Errorf("%q is not a valid CIDR", arg)
		}
		cidr := ipNet.String()
		if c.CIDRs.Contains(cidr) {
			if cidr == arg {
				return errors.Errorf("duplicate subnet %q specified", cidr)
			}
			return errors.Errorf("subnet %q overlaps with %q", arg, cidr)
		}
		c.CIDRs.Add(cidr)
	}

	return nil
}

// Run implements Command.Run.
func (c *CreateCommand) Run(ctx *cmd.Context) error {
	api, err := c.NewAPI()
	if err != nil {
		return errors.Annotate(err, "cannot connect to API server")
	}
	defer api.Close()

	if !c.CIDRs.IsEmpty() {
		// Fetch all subnets to validate the given CIDRs.
		subnets, err := api.AllSubnets()
		if err != nil {
			return errors.Annotate(err, "cannot fetch available subnets")
		}

		// Find which of the given CIDRs match existing ones.
		validCIDRs := set.NewStrings()
		for _, subnet := range subnets {
			validCIDRs.Add(subnet.CIDR)
		}
		diff := c.CIDRs.Difference(validCIDRs)

		if !diff.IsEmpty() {
			// Some given CIDRs are missing.
			subnets := strings.Join(diff.SortedValues(), ", ")
			return errors.Errorf("unknown subnets specified: %s", subnets)
		}
	}

	// Create the new space.
	err = api.CreateSpace(c.Name, c.CIDRs.SortedValues())
	if err != nil {
		return errors.Annotatef(err, "cannot create space %q", c.Name)
	}

	if c.CIDRs.IsEmpty() {
		ctx.Infof("created space %q with no subnets", c.Name)
	} else {
		subnets := strings.Join(c.CIDRs.SortedValues(), ", ")
		ctx.Infof("created space %q with subnets %s", c.Name, subnets)
	}
	return nil
}
