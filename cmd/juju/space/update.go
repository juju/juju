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
	"launchpad.net/gnuflag"
)

// UpdateCommand calls the API to update an existing network space.
type UpdateCommand struct {
	SpaceCommandBase

	Name    string
	NewName string
	CIDRs   set.Strings
}

const updateCommandDoc = `
Replaces the list of associated subnets of the space and/or if --rename is given,
renames the space. Since subnets can only be part of a single space, all specified
subnets (using their CIDRs) "leave" their current space and "enter" the one we're updating.

A network space name can consist of ...
`

func (c *UpdateCommand) SetFlags(f *gnuflag.FlagSet) {
	c.SpaceCommandBase.SetFlags(f)
	f.StringVar(&c.NewName, "rename", "", "the new name for the network space")
}

// Info is defined on the cmd.Command interface.
func (c *UpdateCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "update",
		Args:    "<name> [--rename <new-name>] <CIDR1> [ <CIDR2> ...]",
		Purpose: "update a new network space",
		Doc:     strings.TrimSpace(updateCommandDoc),
	}
}

// Init is defined on the cmd.Command interface. It checks the
// arguments for sanity and sets up the command to run.
func (c *UpdateCommand) Init(args []string) error {
	// Validate given name.
	if len(args) == 0 {
		return errors.New("space name is required")
	}
	c.Name = args[0]
	if !names.IsValidSpace(c.Name) {
		return errors.Errorf("%q is not a valid space name", c.Name)
	}

	if c.NewName != "" && !names.IsValidSpace(c.NewName) {
		return errors.Errorf("%q is not a valid space name", c.NewName)
	}

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

	// Need a name (already checked) and either updated CIDRs and/or a new name
	if c.CIDRs.IsEmpty() && c.NewName == "" {
		return errors.Errorf("new name or updated CIDRs required")
	}

	return nil
}

// Run implements Command.Run.
func (c *UpdateCommand) Run(ctx *cmd.Context) error {
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

	// Update the space.
	err = api.UpdateSpace(c.Name, c.NewName, c.CIDRs.SortedValues())
	if err != nil {
		return errors.Annotatef(err, "cannot update space %q", c.Name)
	}

	var subnets, rename string
	if c.NewName == "" {
		rename = "no rename"
	} else {
		rename = "renamed to " + c.NewName
	}
	if c.CIDRs.IsEmpty() {
		subnets = ", deleted all subnets"
	} else {
		subnets = ", changed subnets to " + strings.Join(c.CIDRs.SortedValues(), ", ")
	}
	ctx.Infof("updated space %q: %s%s", c.Name, rename, subnets)
	return nil
}
