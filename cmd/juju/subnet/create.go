// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnet

import (
	"net"
	"strings"

	"launchpad.net/gnuflag"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils/set"
)

// CreateCommand calls the API to create a new subnet.
type CreateCommand struct {
	SubnetCommandBase

	CIDR      string
	SpaceName string
	Zones     set.Strings
	IsPublic  bool
	IsPrivate bool

	flagSet *gnuflag.FlagSet
}

const createCommandDoc = `
Creates a new subnet with a given CIDR, associated with an existing Juju
network space, and attached to one or more availablility zones. Desired
access for the subnet can be specified using the mutually exclusive flags
--private and --public.

When --private is specified (or no flags are given, as this is the default),
the created subnet will not allow access from outside the environment and
the available address range is only cloud-local.

When --public is specified, the created subnet will support "shadow addresses"
(see "juju help glossary" for the full definition of the term). This means
all machines inside the subnet will have cloud-local addresses configured,
but there will also be a shadow address configured for each machine, so that
the machines can be accessed from outside the environment (similarly to the
automatic public IP addresses supported with AWS VPCs).

This command is only supported on clouds which support creating new subnets
dynamically (i.e. Software Defined Networking or SDN). If you want to make
an existing subnet available for Juju to use, rather than creating a new
one, use the "juju subnet add" command.

Some clouds allow a subnet to span multiple zones, but others do not. It is
an error to try creating a subnet spanning more than one zone if it is not
supported.
`

// Info is defined on the cmd.Command interface.
func (c *CreateCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "create",
		Args:    "<CIDR> <space> <zone1> [<zone2> <zone3> ...] [--public|--private]",
		Purpose: "create a new subnet",
		Doc:     strings.TrimSpace(createCommandDoc),
	}
}

func (c *CreateCommand) SetFlags(f *gnuflag.FlagSet) {
	c.SubnetCommandBase.SetFlags(f)
	f.BoolVar(&c.IsPublic, "public", false, "enable public access with shadow addresses")
	f.BoolVar(&c.IsPrivate, "private", true, "disable public access with shadow addresses")

	// Because SetFlags is called before Parse, we cannot
	// use f.Visit() here to check both flags were not
	// specified at once. So we store the flag set and
	// defer the check to Init().
	c.flagSet = f
}

// Init is defined on the cmd.Command interface. It checks the
// arguments for sanity and sets up the command to run.
func (c *CreateCommand) Init(args []string) error {
	// Validate given CIDR.
	switch len(args) {
	case 0:
		return errors.New("CIDR is required")
	case 1:
		return errors.New("space name is required")
	case 2:
		return errors.New("at least one zone is required")
	}

	givenCIDR := args[0]
	_, ipNet, err := net.ParseCIDR(givenCIDR)
	if err != nil {
		logger.Debugf("cannot parse %q: %v", givenCIDR, err)
		return errors.Errorf("%q is not a valid CIDR", givenCIDR)
	}
	if ipNet.String() != givenCIDR {
		expected := ipNet.String()
		return errors.Errorf("%q is not correctly specified, expected %q", givenCIDR, expected)
	}
	c.CIDR = givenCIDR

	// Validate the space name.
	givenSpace := args[1]
	if !names.IsValidSpace(givenSpace) {
		return errors.Errorf("%q is not a valid space name", givenSpace)
	}
	c.SpaceName = givenSpace

	// Validate any given zones.
	c.Zones = set.NewStrings()
	for _, zone := range args[2:] {
		if c.Zones.Contains(zone) {
			return errors.Errorf("duplicate zone %q specified", zone)
		}
		c.Zones.Add(zone)
	}

	// Ensure --public and --private are not both specified.
	// TODO(dimitern): This is a really awkward way to handle
	// mutually exclusive bool flags and needs to be factored
	// out in a helper if another command needs to do it.
	var publicSet, privateSet bool
	c.flagSet.Visit(func(flag *gnuflag.Flag) {
		switch flag.Name {
		case "public":
			publicSet = true
		case "private":
			privateSet = true
		}
	})
	switch {
	case publicSet && privateSet:
		return errors.Errorf("cannot specify both --public and --private")
	case publicSet:
		c.IsPrivate = false
	case privateSet:
		c.IsPublic = false
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

	if !c.Zones.IsEmpty() {
		// Fetch all zones to validate the given zones.
		zones, err := api.AllZones()
		if err != nil {
			return errors.Annotate(err, "cannot fetch availability zones")
		}

		// Find which of the given CIDRs match existing ones.
		validZones := set.NewStrings()
		for _, zone := range zones {
			validZones.Add(zone)
		}
		diff := c.Zones.Difference(validZones)

		if !diff.IsEmpty() {
			// Some given zones are missing.
			zones := strings.Join(diff.SortedValues(), ", ")
			return errors.Errorf("unknown zones specified: %s", zones)
		}
	}

	// Create the new subnet.
	err = api.CreateSubnet(c.CIDR, c.SpaceName, c.Zones.SortedValues(), c.IsPublic)
	if err != nil {
		return errors.Annotatef(err, "cannot create subnet %q", c.CIDR)
	}

	zones := strings.Join(c.Zones.SortedValues(), ", ")
	accessType := "private"
	if c.IsPublic {
		accessType = "public"
	}
	ctx.Infof(
		"created a %s subnet %q in space %q with zones %s",
		accessType, c.CIDR, c.SpaceName, zones,
	)
	return nil
}
