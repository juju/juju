// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space

import (
	"io"
	"net"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils/featureflag"
	"github.com/juju/utils/set"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/spaces"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/feature"
)

// SpaceAPI defines the necessary API methods needed by the space
// subcommands.
type SpaceAPI interface {
	io.Closer

	// ListSpaces returns all Juju network spaces and their subnets.
	ListSpaces() ([]params.Space, error)

	// CreateSpace creates a new Juju network space, associating the
	// specified subnets with it (optional; can be empty), setting the
	// space and subnets access to public or private.
	CreateSpace(name string, subnetIds []string, public bool) error

	// TODO(dimitern): All of the following api methods should take
	// names.SpaceTag instead of name, the only exceptions are
	// CreateSpace, and RenameSpace as the named space doesn't exist
	// yet.

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

In practice, a space is a collection of related subnets that have no
firewalls between each other, and that have the same ingress and
egress policies. Common examples in company networks are “the dmz” or
“the pci compliant space”. The name of the space suggests that it is a
logical network area which has some specific security characteristics
- hence the “common ingress and egress policy” definition.

All of the addresses in all the subnets in a given space are assumed
to be equally able to connect to one another, and all of them are
assumed to go through the same firewalls (or through the same firewall
rules) for connections into or out of the space. For allocation
purposes, then, putting a service on any address in a space is equally
secure - all the addresses in the space have the same firewall rules
applied to them.

Users create spaces to describe relevant areas of their network (i.e.
DMZ, internal, etc.).

Spaces can be specified via constraints when deploying a service
and/or at add-relation time. Since all subnets in a space are
considered equal, placement of services in a space means placement on
any of the subnets in that space. A machine bound to a space could be
on any one of the subnets, and routable to any other machine in the
space because any subnet in the space can access any other in the same
space.

Initially, there is one space (named "default") which always exists
and "contains" all subnets not associated with another space. However,
since the spaces are defined on the cloud substrate (e.g. using tags
in EC2), there could be pre-existing spaces that get discovered after
bootstrapping a new environment using shared credentials (multiple
users or roles, same substrate). `

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
	spaceCmd.Register(envcmd.Wrap(&ListCommand{}))
	if featureflag.Enabled(feature.PostNetCLIMVP) {
		// The following commands are not part of the MVP.
		spaceCmd.Register(envcmd.Wrap(&RemoveCommand{}))
		spaceCmd.Register(envcmd.Wrap(&UpdateCommand{}))
		spaceCmd.Register(envcmd.Wrap(&RenameCommand{}))
	}

	return spaceCmd
}

// SpaceCommandBase is the base type embedded into all space
// subcommands.
type SpaceCommandBase struct {
	envcmd.EnvCommandBase
	api SpaceAPI
}

// ParseNameAndCIDRs verifies the input args and returns any errors,
// like missing/invalid name or CIDRs (validated when given, but it's
// an error for CIDRs to be empty if cidrsOptional is false).
func ParseNameAndCIDRs(args []string, cidrsOptional bool) (
	name string, CIDRs set.Strings, err error,
) {
	defer errors.DeferredAnnotatef(&err, "invalid arguments specified")

	if len(args) == 0 {
		return "", nil, errors.New("space name is required")
	}
	name, err = CheckName(args[0])
	if err != nil {
		return name, nil, errors.Trace(err)
	}

	CIDRs, err = CheckCIDRs(args[1:], cidrsOptional)
	return name, CIDRs, err
}

// CheckName checks whether name is a valid space name.
func CheckName(name string) (string, error) {
	// Validate given name.
	if !names.IsValidSpace(name) {
		return "", errors.Errorf("%q is not a valid space name", name)
	}
	return name, nil
}

// CheckCIDRs parses the list of strings as CIDRs, checking for
// correct formatting, no duplication and no overlaps. Returns error
// if no CIDRs are provided, unless cidrsOptional is true.
func CheckCIDRs(args []string, cidrsOptional bool) (set.Strings, error) {
	// Validate any given CIDRs.
	CIDRs := set.NewStrings()
	for _, arg := range args {
		_, ipNet, err := net.ParseCIDR(arg)
		if err != nil {
			logger.Debugf("cannot parse %q: %v", arg, err)
			return CIDRs, errors.Errorf("%q is not a valid CIDR", arg)
		}
		cidr := ipNet.String()
		if CIDRs.Contains(cidr) {
			if cidr == arg {
				return CIDRs, errors.Errorf("duplicate subnet %q specified", cidr)
			}
			return CIDRs, errors.Errorf("subnet %q overlaps with %q", arg, cidr)
		}
		CIDRs.Add(cidr)
	}

	if CIDRs.IsEmpty() && !cidrsOptional {
		return CIDRs, errors.New("CIDRs required but not provided")
	}

	return CIDRs, nil
}

// mvpAPIShim forwards SpaceAPI methods to the real API facade for
// implemented methods only. Tested with a feature test only.
type mvpAPIShim struct {
	SpaceAPI

	apiState api.Connection
	facade   *spaces.API
}

func (m *mvpAPIShim) Close() error {
	return m.apiState.Close()
}

func (m *mvpAPIShim) CreateSpace(name string, subnetIds []string, public bool) error {
	return m.facade.CreateSpace(name, subnetIds, public)
}

func (m *mvpAPIShim) ListSpaces() ([]params.Space, error) {
	return m.facade.ListSpaces()
}

// NewAPI returns a SpaceAPI for the root api endpoint that the
// environment command returns.
func (c *SpaceCommandBase) NewAPI() (SpaceAPI, error) {
	if c.api != nil {
		// Already created.
		return c.api, nil
	}
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}

	// This is tested with a feature test.
	shim := &mvpAPIShim{
		apiState: root,
		facade:   spaces.NewAPI(root),
	}
	return shim, nil
}

type RunOnAPI func(api SpaceAPI, ctx *cmd.Context) error

func (c *SpaceCommandBase) RunWithAPI(ctx *cmd.Context, toRun RunOnAPI) error {
	api, err := c.NewAPI()
	if err != nil {
		return errors.Annotate(err, "cannot connect to the API server")
	}
	defer api.Close()
	return toRun(api, ctx)
}
