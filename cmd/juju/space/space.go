// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space

import (
	"io"
	"net"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/set"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/spaces"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
)

// SpaceAPI defines the necessary API methods needed by the space
// subcommands.
type SpaceAPI interface {
	io.Closer

	// ListSpaces returns all Juju network spaces and their subnets.
	ListSpaces() ([]params.Space, error)

	// AddSpace adds a new Juju network space, associating the
	// specified subnets with it (optional; can be empty), setting the
	// space and subnets access to public or private.
	AddSpace(name string, subnetIds []string, public bool) error

	// TODO(dimitern): All of the following api methods should take
	// names.SpaceTag instead of name, the only exceptions are
	// AddSpace, and RenameSpace as the named space doesn't exist
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

// SpaceCommandBase is the base type embedded into all space
// subcommands.
type SpaceCommandBase struct {
	modelcmd.ModelCommandBase
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
	return name, CIDRs, errors.Trace(err)
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

func (m *mvpAPIShim) AddSpace(name string, subnetIds []string, public bool) error {
	return m.facade.CreateSpace(name, subnetIds, public)
}

func (m *mvpAPIShim) ListSpaces() ([]params.Space, error) {
	return m.facade.ListSpaces()
}

// NewAPI returns a SpaceAPI for the root api endpoint that the
// environment command returns.
func (c *SpaceCommandBase) NewAPI() (SpaceAPI, error) {
	if c.api != nil {
		// Already addd.
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
