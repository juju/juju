// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space

import (
	"context"
	"io"
	"net"

	"github.com/juju/cmd/v4"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/client/spaces"
	"github.com/juju/juju/api/client/subnets"
	"github.com/juju/juju/cmd/modelcmd"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/rpc/params"
)

// SpaceAPI defines the necessary API methods needed by the space
// subcommands.
type SpaceAPI interface {
	// ListSpaces returns all Juju network spaces and their subnets.
	ListSpaces(ctx context.Context) ([]params.Space, error)

	// AddSpace adds a new Juju network space, associating the
	// specified subnets with it (optional; can be empty), setting the
	// space and subnets access to public or private.
	AddSpace(ctx context.Context, name string, subnetIds []string, public bool) error

	// TODO(dimitern): All of the following api methods should take
	// names.SpaceTag instead of name, the only exceptions are
	// AddSpace, and RenameSpace as the named space doesn't exist
	// yet.

	// RemoveSpace removes an existing Juju network space, transferring
	// any associated subnets to the default space.
	RemoveSpace(ctx context.Context, name string, force bool, dryRun bool) (params.RemoveSpaceResult, error)

	// RenameSpace changes the name of the space.
	RenameSpace(ctx context.Context, name, newName string) error

	// ReloadSpaces fetches spaces and subnets from substrate
	ReloadSpaces(ctx context.Context) error

	// ShowSpace fetches space information.
	ShowSpace(ctx context.Context, name string) (params.ShowSpaceResult, error)

	// MoveSubnets ensures that the input subnets are in the input space.
	MoveSubnets(context.Context, names.SpaceTag, []names.SubnetTag, bool) (params.MoveSubnetsResult, error)
}

// SubnetAPI defines the necessary API methods needed by the subnet subcommands.
type SubnetAPI interface {

	// SubnetsByCIDR returns the collection of subnets matching each CIDR in the input.
	SubnetsByCIDR(context.Context, []string) ([]params.SubnetsResult, error)
}

// API defines the contract for requesting the API facades.
type API interface {
	io.Closer

	SpaceAPI
	SubnetAPI
}

var logger = internallogger.GetLogger("juju.cmd.juju.space")

// SpaceCommandBase is the base type embedded into all space subcommands.
type SpaceCommandBase struct {
	modelcmd.ModelCommandBase
	modelcmd.IAASOnlyCommand
	api API
}

// ParseNameAndCIDRs verifies the input args and returns any errors,
// like missing/invalid name or CIDRs (validated when given, but it's
// an error for CIDRs to be empty if cidrsOptional is false).
func ParseNameAndCIDRs(args []string, cidrsOptional bool) (
	name string, cidrs set.Strings, err error,
) {
	defer errors.DeferredAnnotatef(&err, "invalid arguments specified")

	if len(args) == 0 {
		return "", nil, errors.New("space name is required")
	}
	name, err = CheckName(args[0])
	if err != nil {
		return name, nil, errors.Trace(err)
	}

	cidrs, err = CheckCIDRs(args[1:], cidrsOptional)
	return name, cidrs, errors.Trace(err)
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
	cidrs := set.NewStrings()
	for _, arg := range args {
		_, ipNet, err := net.ParseCIDR(arg)
		if err != nil {
			logger.Debugf(context.TODO(), "cannot parse %q: %v", arg, err)
			return cidrs, errors.Errorf("%q is not a valid CIDR", arg)
		}
		cidr := ipNet.String()
		if cidrs.Contains(cidr) {
			if cidr == arg {
				return cidrs, errors.Errorf("duplicate subnet %q specified", cidr)
			}
			return cidrs, errors.Errorf("subnet %q overlaps with %q", arg, cidr)
		}
		cidrs.Add(cidr)
	}

	if cidrs.IsEmpty() && !cidrsOptional {
		return cidrs, errors.New("CIDRs required but not provided")
	}

	return cidrs, nil
}

// APIShim forwards SpaceAPI methods to the real API facade for
// implemented methods only.
type APIShim struct {
	SpaceAPI

	apiState  api.Connection
	spaceAPI  *spaces.API
	subnetAPI *subnets.API
}

func (m *APIShim) Close() error {
	return m.apiState.Close()
}

// AddSpace adds a new Juju network space, associating the
// specified subnets with it (optional; can be empty), setting the
// space and subnets access to public or private.
func (m *APIShim) AddSpace(ctx context.Context, name string, subnetIds []string, public bool) error {
	return m.spaceAPI.CreateSpace(ctx, name, subnetIds, public)
}

// ListSpaces returns all Juju network spaces and their subnets.
func (m *APIShim) ListSpaces(ctx context.Context) ([]params.Space, error) {
	return m.spaceAPI.ListSpaces(ctx)
}

// ReloadSpaces fetches spaces and subnets from substrate
func (m *APIShim) ReloadSpaces(ctx context.Context) error {
	return m.spaceAPI.ReloadSpaces(ctx)
}

// RemoveSpace removes an existing Juju network space, transferring
// any associated subnets to the default space.
func (m *APIShim) RemoveSpace(ctx context.Context, name string, force bool, dryRun bool) (params.RemoveSpaceResult, error) {
	return m.spaceAPI.RemoveSpace(ctx, name, force, dryRun)
}

// RenameSpace changes the name of the space.
func (m *APIShim) RenameSpace(ctx context.Context, oldName, newName string) error {
	return m.spaceAPI.RenameSpace(ctx, oldName, newName)
}

// ShowSpace fetches space information.
func (m *APIShim) ShowSpace(ctx context.Context, name string) (params.ShowSpaceResult, error) {
	return m.spaceAPI.ShowSpace(ctx, name)
}

// MoveSubnets ensures that the input subnets are in the input space.
func (m *APIShim) MoveSubnets(ctx context.Context, space names.SpaceTag, subnets []names.SubnetTag, force bool) (params.MoveSubnetsResult, error) {
	return m.spaceAPI.MoveSubnets(ctx, space, subnets, force)
}

// SubnetsByCIDR returns the collection of subnets matching each CIDR in the input.
func (m *APIShim) SubnetsByCIDR(ctx context.Context, cidrs []string) ([]params.SubnetsResult, error) {
	return m.subnetAPI.SubnetsByCIDR(ctx, cidrs)
}

// NewAPI returns a API for the root api endpoint that the
// environment command returns.
func (c *SpaceCommandBase) NewAPI(ctx context.Context) (API, error) {
	if c.api != nil {
		// Already added.
		return c.api, nil
	}
	root, err := c.NewAPIRoot(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// This is tested with a feature test.
	shim := &APIShim{
		apiState:  root,
		spaceAPI:  spaces.NewAPI(root),
		subnetAPI: subnets.NewAPI(root),
	}
	return shim, nil
}

type RunOnAPI func(api API, ctx *cmd.Context) error

func (c *SpaceCommandBase) RunWithAPI(ctx *cmd.Context, toRun RunOnAPI) error {
	api, err := c.NewAPI(ctx)
	if err != nil {
		return errors.Annotate(err, "cannot connect to the API server")
	}
	defer api.Close()
	return toRun(api, ctx)
}

type RunOnSpaceAPI func(api SpaceAPI, ctx *cmd.Context) error

func (c *SpaceCommandBase) RunWithSpaceAPI(ctx *cmd.Context, toRun RunOnSpaceAPI) error {
	api, err := c.NewAPI(ctx)
	if err != nil {
		return errors.Annotate(err, "cannot connect to the API server")
	}
	defer api.Close()
	return toRun(api, ctx)
}

// SubnetInfo is a source-agnostic representation of a subnet.
// It may originate from state, or from a provider.
type SubnetInfo struct {
	// CIDR of the network, in 123.45.67.89/24 format.
	CIDR string `json:"cidr" yaml:"cidr"`

	// ProviderId is a provider-specific subnet ID.
	ProviderId string `json:"provider-id,omitempty" yaml:"provider-id,omitempty"`

	// ProviderSpaceId holds the provider ID of the space associated
	// with this subnet. Can be empty if not supported.
	ProviderSpaceId string `json:"provider-space-id,omitempty" yaml:"provider-space-id,omitempty"`

	// ProviderNetworkId holds the provider ID of the network
	// containing this subnet, for example VPC id for EC2.
	ProviderNetworkId string `json:"provider-network-id,omitempty" yaml:"provider-network-id,omitempty"`

	// VLANTag needs to be between 1 and 4094 for VLANs and 0 for
	// normal networks. It's defined by IEEE 802.1Q standard, and used
	// to define a VLAN network. For more information, see:
	// http://en.wikipedia.org/wiki/IEEE_802.1Q.
	VLANTag int `json:"vlan-tag" yaml:"vlan-tag"`

	// AvailabilityZones describes which availability zones this
	// subnet is in. It can be empty if the provider does not support
	// availability zones.
	AvailabilityZones []string `json:"zones,omitempty" yaml:"zones,omitempty"`

	// SpaceID is the id of the space the subnet is associated with.
	// Default value should be AlphaSpaceId. It can be empty if
	// the subnet is returned from an networkingEnviron. SpaceID is
	// preferred over SpaceName in state and non networkingEnviron use.
	SpaceID string `json:"space-id,omitempty" yaml:"space-id,omitempty"`

	// SpaceName is the name of the space the subnet is associated with.
	// An empty string indicates it is part of the AlphaSpaceName OR
	// if the SpaceID is set. Should primarily be used in an networkingEnviron.
	SpaceName string `json:"space-name,omitempty" yaml:"space-name,omitempty"`
}

// SpaceInfo defines a network space.
type SpaceInfo struct {
	// ID is the unique identifier for the space.
	ID string `json:"id" yaml:"id"`

	// Name is the name of the space.
	// It is used by operators for identifying a space and should be unique.
	Name string `json:"name" yaml:"name"`

	// ProviderId is the provider's unique identifier for the space,
	// such as used by MAAS.
	ProviderId string `json:"provider-id,omitempty" yaml:"provider-id,omitempty"`

	// Subnets are the subnets that have been grouped into this network space.
	Subnets []SubnetInfo `json:"subnets" yaml:"subnets"`
}

// FanCIDRs describes the subnets relevant to a fan network.
type FanCIDRs struct {
	// FanLocalUnderlay is the CIDR of the local underlying fan network.
	// It allows easy identification of the device the fan is running on.
	FanLocalUnderlay string `json:"fan-local-underlay" yaml:"fan-local-underlay"`

	// FanOverlay is the CIDR of the complete fan setup.
	FanOverlay string `json:"fan-overlay" yaml:"fan-overlay"`
}

// convertEntitiesToStringAndSkipModel skips the modelTag as this will be used on another place.
func convertEntitiesToStringAndSkipModel(entities []params.Entity) ([]string, error) {
	var outputString []string
	for _, ent := range entities {
		tag, err := names.ParseTag(ent.Tag)
		if err != nil {
			return nil, err
		}
		if tag.Kind() == names.ModelTagKind {
			continue
		} else {
			outputString = append(outputString, tag.Id())
		}
	}
	return outputString, nil
}

func hasModelConstraint(entities []params.Entity) (bool, error) {
	for _, entity := range entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			return false, err
		}
		if tag.Kind() == names.ModelTagKind {
			return true, nil
		}
	}
	return false, nil
}
