// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnet

import (
	"io"
	"net"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
)

// SubnetAPI defines the necessary API methods needed by the subnet
// subcommands.
type SubnetAPI interface {
	io.Closer

	// AllZones returns all availability zones known to Juju.
	AllZones() ([]string, error)

	// AllSpaces returns all Juju network spaces.
	AllSpaces() ([]names.Tag, error)

	// CreateSubnet creates a new Juju subnet.
	CreateSubnet(subnetCIDR string, spaceTag names.SpaceTag, zones []string, isPublic bool) error

	// AddSubnet adds an existing subnet to Juju.
	AddSubnet(cidr, id string, spaceTag names.SpaceTag, zones []string) error

	// RemoveSubnet marks an existing subnet as no longer used, which
	// will cause it to get removed at some point after all its
	// related entites are cleaned up. It will fail if the subnet is
	// still in use by any machines.
	RemoveSubnet(subnetCIDR string) error

	// ListSubnets returns information about subnets known to Juju,
	// optionally filtered by space and/or zone (both can be empty).
	ListSubnets(withSpace *names.SpaceTag, withZone string) ([]params.Subnet, error)
}

var logger = loggo.GetLogger("juju.cmd.juju.subnet")

const commandDoc = `
"juju subnet" provides commands to manage Juju subnets. In Juju, a
subnet is a logical address range, a subdivision of a network, defined
by the subnet's Classless Inter-Domain Routing (CIDR) range, like
10.10.0.0/24 or 2001:db8::/32. Alternatively, subnets can be
identified uniquely by their provider-specific identifier
(ProviderId), if the provider supports that. Subnets have two kinds of
supported access: "public" (using shadow addresses) or "private"
(using cloud-local addresses, this is the default). For more
information about subnets and shadow addresses, please refer to Juju's
glossary help topics ("juju help glossary"). `

// NewSuperCommand creates the "subnet" supercommand and registers the
// subcommands that it supports.
func NewSuperCommand() cmd.Command {
	subnetCmd := cmd.NewSuperCommand(cmd.SuperCommandParams{
		Name:        "subnet",
		Doc:         strings.TrimSpace(commandDoc),
		UsagePrefix: "juju",
		Purpose:     "manage subnets",
	})
	subnetCmd.Register(envcmd.Wrap(&CreateCommand{}))
	subnetCmd.Register(envcmd.Wrap(&AddCommand{}))
	subnetCmd.Register(envcmd.Wrap(&RemoveCommand{}))
	subnetCmd.Register(envcmd.Wrap(&ListCommand{}))

	return subnetCmd
}

// SubnetCommandBase is the base type embedded into all subnet
// subcommands.
type SubnetCommandBase struct {
	envcmd.EnvCommandBase
	api SubnetAPI
}

// NewAPI returns a SubnetAPI for the root api endpoint that the
// environment command returns.
func (c *SubnetCommandBase) NewAPI() (SubnetAPI, error) {
	// TODO(dimitern): Change this once the API is implemented.

	if c.api != nil {
		// Already created.
		return c.api, nil
	}

	return nil, errors.New("API not implemented yet!")
}

// Common errors shared between subcommands.
var (
	errNoCIDR     = errors.New("CIDR is required")
	errNoCIDROrID = errors.New("either CIDR or provider ID is required")
	errNoSpace    = errors.New("space name is required")
	errNoZones    = errors.New("at least one zone is required")
)

// CheckArgs is a helper used to validate the number of arguments
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
func (s *SubnetCommandBase) ValidateCIDR(given string, strict bool) (string, error) {
	_, ipNet, err := net.ParseCIDR(given)
	if err != nil {
		logger.Debugf("cannot parse CIDR %q: %v", given, err)
		return "", errors.Errorf("%q is not a valid CIDR", given)
	}
	if strict && given != ipNet.String() {
		expected := ipNet.String()
		return "", errors.Errorf("%q is not correctly specified, expected %q", given, expected)
	}
	return ipNet.String(), nil
}

// ValidateSpace parses given and returns an error if it's not a valid
// space name, otherwise returns the parsed tag and no error.
func (s *SubnetCommandBase) ValidateSpace(given string) (names.SpaceTag, error) {
	if !names.IsValidSpace(given) {
		return names.SpaceTag{}, errors.Errorf("%q is not a valid space name", given)
	}
	return names.NewSpaceTag(given), nil
}
