// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space

import (
	"io"
	"net"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/network"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils/set"
)

// SpaceAPI defines the necessary API methods needed by the space
// subcommands.
type SpaceAPI interface {
	io.Closer

	// AllSubnets returns all subnets known to Juju.
	AllSubnets() ([]network.SubnetInfo, error)

	// CreateSpace creates a new Juju network space, associating the
	// specified subnets with it (optional; can be empty).
	CreateSpace(name string, subnetIds []string) error

	// RemoveSpace removes an existing Juju network space, transferring
	// any associated subnets to the default space.
	RemoveSpace(name string) error

	// UpdateSpace changes the associated subnets for an existing space with
	// the given name. The list of subnets must contain at least one entry.
	UpdateSpace(name string, subnetIds []string) error

	// RenameSpace changes the name of the space.
	RenameSpace(name, newName string) error
}

var logger = loggo.GetLogger("juju.cmd.juju.space")

const commandDoc = `
"juju space" provides commands to manage Juju network spaces.

A space is a security subdivision of a network.

In practice, a space is a collection of related subnets that have no firewalls between each other, and that have the
same ingress and egress policies. Common examples in company networks are “the dmz” or “the pci compliant space”. The
name of the space suggests that it is a logical network area which has some specific security characteristics - hence
the “common ingress and egress policy” definition.

All of the addresses in all the subnets in a given space are assumed to be equally able to connect to one another, and
all of them are assumed to go through the same firewalls (or through the same firewall rules) for connections into or
out of the space. For allocation purposes, then, putting a service on any address in a space is equally secure - all the
addresses in the space have the same firewall rules applied to them.

Users create spaces to describe relevant areas of their network (i.e. DMZ, internal, etc.).

Spaces can be specified via constraints when deploying a service and/or at add-relation time. Since all subnets in a
space are considered equal, placement of services in a space means placement on any of the subnets in that space. A
machine bound to a space could be on any one of the subnets, and routable to any other machine in the space because any
subnet in the space can access any other in  the same space.

Initially, there is one space (named "default") which always exists and "contains" all subnets not associated with
another space. However, since the spaces are defined on the cloud substrate (e.g. using tags in EC2), there could be
pre-existing spaces that get discovered after bootstrapping a new environment using shared credentials (multiple users
or roles, same substrate).
`

// NewSuperCommand creates the "space" supercommand and registers the
// subcommands that it supports.
func NewSuperCommand() cmd.Command {
	spaceCmd := cmd.NewSuperCommand(cmd.SuperCommandParams{
		Name:        "space",
		Doc:         strings.TrimSpace(commandDoc),
		UsagePrefix: "juju",
		Purpose:     "manage network spaces",
	})
	spaceCmd.Register(envcmd.Wrap(&CreateCommand{}))
	spaceCmd.Register(envcmd.Wrap(&RemoveCommand{}))
	spaceCmd.Register(envcmd.Wrap(&UpdateCommand{}))
	spaceCmd.Register(envcmd.Wrap(&RenameCommand{}))

	return spaceCmd
}

// SpaceCommandBase is the base type embedded into all space
// subcommands.
type SpaceCommandBase struct {
	envcmd.EnvCommandBase
	api   SpaceAPI
	Name  string
	CIDRs set.Strings
}

func (c *SpaceCommandBase) ParseNameAndCIDRs(args []string) error {
	if len(args) == 0 {
		return errors.New("space name is required")
	}
	err := c.ParseName(args[0])
	if err != nil {
		return err
	}

	if len(args) == 1 {
		return errors.New("CIDRs required but not provided")
	}
	err = c.ParseCIDRs(args[1:])
	if err != nil {
		return err
	}
	return nil
}

// Init is defined on the cmd.Command interface. It checks the
// arguments for sanity and sets up the command to run.
func (c *SpaceCommandBase) ParseName(name string) error {
	// Validate given name.
	if !names.IsValidSpace(name) {
		return errors.Errorf("%q is not a valid space name", name)
	}
	c.Name = name

	return nil
}

// ParseCIDRs parses the list of strings as CIDRs, checking for correct formatting,
// no duplication and no overlaps.
func (c *SpaceCommandBase) ParseCIDRs(args []string) error {
	// Validate any given CIDRs.
	c.CIDRs = set.NewStrings()
	for _, arg := range args {
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
	if c.CIDRs.IsEmpty() {
		return errors.Errorf("CIDRs required but not provided")
	}

	return nil
}

// NewAPI returns a SpaceAPI for the root api endpoint that the
// environment command returns.
func (c *SpaceCommandBase) NewAPI() (SpaceAPI, error) {
	// TODO(dimitern): Change this once the API is implemented.

	if c.api != nil {
		// Already created.
		return c.api, nil
	}

	return nil, errors.New("API not implemented yet!")
}

func (c *SpaceCommandBase) SubnetsExist(api SpaceAPI) error {
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

	return nil
}

type RunOnApi func(api SpaceAPI, ctx *cmd.Context) error

func (c *SpaceCommandBase) RunWithAPI(ctx *cmd.Context, toRun RunOnApi) error {
	api, err := c.NewAPI()
	if err != nil {
		return errors.Annotate(err, "cannot connect to API server")
	}
	defer api.Close()

	if !c.CIDRs.IsEmpty() {
		// Check that c.CIDRs contains subnets that Juju already knows about
		if err := c.SubnetsExist(api); err != nil {
			return err
		}
	}

	return toRun(api, ctx)
}
