// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnet

import (
	"io"
	"net"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/subnets"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/network"
)

// SubnetAPI defines the necessary API methods needed by the subnet
// subcommands.
type SubnetAPI interface {
	io.Closer

	// AddSubnet adds an existing subnet to Juju.
	AddSubnet(cidr names.SubnetTag, id network.Id, spaceTag names.SpaceTag, zones []string) error

	// ListSubnets returns information about subnets known to Juju,
	// optionally filtered by space and/or zone (both can be empty).
	ListSubnets(withSpace *names.SpaceTag, withZone string) ([]params.Subnet, error)

	// AllZones returns all availability zones known to Juju.
	AllZones() ([]string, error)

	// AllSpaces returns all Juju network spaces.
	AllSpaces() ([]names.Tag, error)

	// CreateSubnet creates a new Juju subnet.
	CreateSubnet(subnetCIDR names.SubnetTag, spaceTag names.SpaceTag, zones []string, isPublic bool) error

	// RemoveSubnet marks an existing subnet as no longer used, which
	// will cause it to get removed at some point after all its
	// related entites are cleaned up. It will fail if the subnet is
	// still in use by any machines.
	RemoveSubnet(subnetCIDR names.SubnetTag) error
}

// mvpAPIShim forwards SubnetAPI methods to the real API facade for
// implemented methods only. Tested with a feature test only.
type mvpAPIShim struct {
	SubnetAPI

	apiState api.Connection
	facade   *subnets.API
}

func (m *mvpAPIShim) Close() error {
	return m.apiState.Close()
}

func (m *mvpAPIShim) AddSubnet(cidr names.SubnetTag, id network.Id, spaceTag names.SpaceTag, zones []string) error {
	return m.facade.AddSubnet(cidr, id, spaceTag, zones)
}

func (m *mvpAPIShim) ListSubnets(withSpace *names.SpaceTag, withZone string) ([]params.Subnet, error) {
	return m.facade.ListSubnets(withSpace, withZone)
}

var logger = loggo.GetLogger("juju.cmd.juju.subnet")

// SubnetCommandBase is the base type embedded into all subnet
// subcommands.
type SubnetCommandBase struct {
	modelcmd.ModelCommandBase
	modelcmd.IAASOnlyCommand
	api SubnetAPI
}

// NewAPI returns a SubnetAPI for the root api endpoint that the
// environment command returns.
func (c *SubnetCommandBase) NewAPI() (SubnetAPI, error) {
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
		facade:   subnets.NewAPI(root),
	}
	return shim, nil
}

type RunOnAPI func(api SubnetAPI, ctx *cmd.Context) error

func (c *SubnetCommandBase) RunWithAPI(ctx *cmd.Context, toRun RunOnAPI) error {
	api, err := c.NewAPI()
	if err != nil {
		return errors.Annotate(err, "cannot connect to the API server")
	}
	defer api.Close()
	return toRun(api, ctx)
}

// Common errors shared between subcommands.
var (
	errNoCIDR     = errors.New("CIDR is required")
	errNoCIDROrID = errors.New("either CIDR or provider ID is required")
	errNoSpace    = errors.New("space name is required")
	errNoZones    = errors.New("at least one zone is required")
)

// CheckNumArgs is a helper used to validate the number of arguments
// passed to Init(). If the number of arguments is X, errors[X] (if
// set) will be returned, otherwise no error occurs.
func (s *SubnetCommandBase) CheckNumArgs(args []string, errors []error) error {
	for num, err := range errors {
		if len(args) == num {
			return err
		}
	}
	return nil
}

// ValidateCIDR parses given and returns an error if it's not valid.
// If the CIDR is incorrectly specified (e.g. 10.10.10.0/16 instead of
// 10.10.0.0/16) and strict is false, the correctly parsed CIDR in the
// expected format is returned instead without an error. Otherwise,
// when strict is true and given is incorrectly formatted, an error
// will be returned.
func (s *SubnetCommandBase) ValidateCIDR(given string, strict bool) (names.SubnetTag, error) {
	_, ipNet, err := net.ParseCIDR(given)
	if err != nil {
		logger.Debugf("cannot parse CIDR %q: %v", given, err)
		return names.SubnetTag{}, errors.Errorf("%q is not a valid CIDR", given)
	}
	if strict && given != ipNet.String() {
		expected := ipNet.String()
		return names.SubnetTag{}, errors.Errorf("%q is not correctly specified, expected %q", given, expected)
	}
	// Already validated, so shouldn't error here.
	return names.NewSubnetTag(ipNet.String()), nil
}

// ValidateSpace parses given and returns an error if it's not a valid
// space name, otherwise returns the parsed tag and no error.
func (s *SubnetCommandBase) ValidateSpace(given string) (names.SpaceTag, error) {
	if !names.IsValidSpace(given) {
		return names.SpaceTag{}, errors.Errorf("%q is not a valid space name", given)
	}
	return names.NewSpaceTag(given), nil
}
