// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnets

import (
	"fmt"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/networkingcommon"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	providercommon "github.com/juju/juju/provider/common"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.subnets")

// Backend contains the state methods used in this package.
type Backing interface {
	environs.EnvironConfigGetter

	// AvailabilityZones returns all cached availability zones (i.e.
	// not from the provider, but in state).
	AvailabilityZones() ([]providercommon.AvailabilityZone, error)

	// SetAvailabilityZones replaces the cached list of availability
	// zones with the given zones.
	SetAvailabilityZones([]providercommon.AvailabilityZone) error

	// AddSubnet creates a backing subnet for an existing subnet.
	AddSubnet(networkingcommon.BackingSubnetInfo) (networkingcommon.BackingSubnet, error)

	// AllSubnets returns all backing subnets.
	AllSubnets() ([]networkingcommon.BackingSubnet, error)

	// SubnetByCIDR returns a unique subnet based on the input CIDR.
	SubnetByCIDR(cidr string) (networkingcommon.BackingSubnet, error)

	// SubnetsByCIDR returns any subnets with the input CIDR.
	SubnetsByCIDR(cidr string) ([]networkingcommon.BackingSubnet, error)

	// AllSpaces returns all known Juju network spaces.
	AllSpaces() ([]networkingcommon.BackingSpace, error)

	// ModelTag returns the tag of the model this state is associated to.
	ModelTag() names.ModelTag
}

// APIv2 provides the subnets API facade for versions < 3.
type APIv2 struct {
	*APIv3
}

// APIv3 provides the subnets API facade for versions 3.
type APIv3 struct {
	*API
}

// API provides the subnets API facade for version 4.
type API struct {
	backing    Backing
	resources  facade.Resources
	authorizer facade.Authorizer
	context    context.ProviderCallContext
}

// NewAPIv2 is a wrapper that creates a V2 subnets API.
func NewAPIv2(st *state.State, res facade.Resources, auth facade.Authorizer) (*APIv2, error) {
	api, err := NewAPIv3(st, res, auth)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv2{api}, nil
}

// NewAPIv3 is a wrapper that creates a V3 subnets API.
func NewAPIv3(st *state.State, res facade.Resources, auth facade.Authorizer) (*APIv3, error) {
	api, err := NewAPI(st, res, auth)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv3{api}, nil
}

// NewAPI creates a new Subnets API server-side facade with a
// state.State backing.
func NewAPI(st *state.State, res facade.Resources, auth facade.Authorizer) (*API, error) {
	stateShim, err := NewStateShim(st)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return newAPIWithBacking(stateShim, context.CallContext(st), res, auth)
}

func (api *API) checkCanRead() error {
	canRead, err := api.authorizer.HasPermission(permission.ReadAccess, api.backing.ModelTag())
	if err != nil {
		return errors.Trace(err)
	}
	if !canRead {
		return common.ServerError(common.ErrPerm)
	}
	return nil
}

func (api *API) checkCanWrite() error {
	canWrite, err := api.authorizer.HasPermission(permission.WriteAccess, api.backing.ModelTag())
	if err != nil {
		return errors.Trace(err)
	}
	if !canWrite {
		return common.ServerError(common.ErrPerm)
	}
	return nil
}

// newAPIWithBacking creates a new server-side Subnets API facade with
// a common.NetworkBacking
func newAPIWithBacking(backing Backing, ctx context.ProviderCallContext, resources facade.Resources, authorizer facade.Authorizer) (*API, error) {
	// Only clients can access the Subnets facade.
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}
	return &API{
		backing:    backing,
		resources:  resources,
		authorizer: authorizer,
		context:    ctx,
	}, nil
}

// AllZones returns all availability zones known to Juju. If a
// zone is unusable, unavailable, or deprecated the Available
// field will be false.
func (api *API) AllZones() (params.ZoneResults, error) {
	if err := api.checkCanRead(); err != nil {
		return params.ZoneResults{}, err
	}
	return allZones(api.context, api.backing)
}

// AllSpaces returns the tags of all network spaces known to Juju.
// This is not recruited by any Juju client code and should not actually exist.
// It is preserved for versions 3 and below of the facade,
// but blanked for later versions to ensure it is not used.
func (api *APIv3) AllSpaces() (params.SpaceResults, error) {
	if err := api.checkCanRead(); err != nil {
		return params.SpaceResults{}, err
	}

	var results params.SpaceResults

	spaces, err := api.backing.AllSpaces()
	if err != nil {
		return results, errors.Trace(err)
	}

	results.Results = make([]params.SpaceResult, len(spaces))
	for i, space := range spaces {
		tag := names.NewSpaceTag(space.Name())
		results.Results[i].Tag = tag.String()
	}
	return results, nil
}

// AllSpaces does not belong on this API facade.
// If you need it, use the spaces facade.
func (api *API) AllSpaces(_, _ struct{}) {}

// AddSubnets adds existing subnets to Juju.
func (api *API) AddSubnets(args params.AddSubnetsParams) (params.ErrorResults, error) {
	if err := api.checkCanWrite(); err != nil {
		return params.ErrorResults{}, err
	}
	return api.addSubnets(args)
}

// AddSubnets adds existing subnets to Juju.
// Args are converted to the new form for compatibility.
func (api *APIv2) AddSubnets(args params.AddSubnetsParamsV2) (params.ErrorResults, error) {
	if err := api.checkCanWrite(); err != nil {
		return params.ErrorResults{}, err
	}
	newArgs, errIndex, err := convertToAddSubnetsParams(args)
	if err != nil {
		results := params.ErrorResults{
			Results: make([]params.ErrorResult, len(args.Subnets)),
		}
		results.Results[errIndex].Error = common.ServerError(err)
	}
	return api.addSubnets(newArgs)
}

func (api *API) addSubnets(args params.AddSubnetsParams) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Subnets)),
	}

	if len(args.Subnets) == 0 {
		return results, nil
	}

	cache := NewAddSubnetsCache(api.backing)
	for i, arg := range args.Subnets {
		err := addOneSubnet(api.context, api.backing, arg, cache)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
		}
	}
	return results, nil
}

// ListSubnets returns the matching subnets after applying
// optional filters.
func (api *API) ListSubnets(args params.SubnetsFilters) (results params.ListSubnetsResults, err error) {
	if err := api.checkCanRead(); err != nil {
		return params.ListSubnetsResults{}, err
	}

	subs, err := api.backing.AllSubnets()
	if err != nil {
		return results, errors.Trace(err)
	}

	var spaceFilter string
	if args.SpaceTag != "" {
		tag, err := names.ParseSpaceTag(args.SpaceTag)
		if err != nil {
			return results, errors.Trace(err)
		}
		spaceFilter = tag.Id()
	}
	zoneFilter := args.Zone

	for _, subnet := range subs {
		if spaceFilter != "" && subnet.SpaceName() != spaceFilter {
			logger.Tracef(
				"filtering subnet %q from space %q not matching filter %q",
				subnet.CIDR(), subnet.SpaceName(), spaceFilter,
			)
			continue
		}
		zoneSet := set.NewStrings(subnet.AvailabilityZones()...)
		if zoneFilter != "" && !zoneSet.IsEmpty() && !zoneSet.Contains(zoneFilter) {
			logger.Tracef(
				"filtering subnet %q with zones %v not matching filter %q",
				subnet.CIDR(), subnet.AvailabilityZones(), zoneFilter,
			)
			continue
		}

		results.Results = append(results.Results, networkingcommon.BackingSubnetToParamsSubnet(subnet))
	}
	return results, nil
}

// SubnetsByCIDR returns the collection of subnets matching each CIDR in the input.
func (api *API) SubnetsByCIDR(arg params.CIDRParams) (params.SubnetsResults, error) {
	result := params.SubnetsResults{}

	if err := api.checkCanRead(); err != nil {
		return result, err
	}

	results := make([]params.SubnetsResult, len(arg.CIDRS))
	for i, cidr := range arg.CIDRS {
		if !network.IsValidCIDR(cidr) {
			results[i].Error = common.ServerError(errors.NotValidf("CIDR %q", cidr))
			continue
		}

		subnets, err := api.backing.SubnetsByCIDR(cidr)
		if err != nil {
			results[i].Error = common.ServerError(err)
			continue
		}

		subnetResults := make([]params.SubnetV2, len(subnets))
		for j, subnet := range subnets {
			subnetResults[j] = networkingcommon.BackingSubnetToParamsSubnetV2(subnet)
		}
		results[i].Subnets = subnetResults
	}
	result.Results = results
	return result, nil
}

func convertToAddSubnetsParams(old params.AddSubnetsParamsV2) (params.AddSubnetsParams, int, error) {
	subnetsParams := params.AddSubnetsParams{
		Subnets: make([]params.AddSubnetParams, len(old.Subnets)),
	}
	for i, oldSubnet := range old.Subnets {
		split := strings.Split(oldSubnet.SubnetTag, "-")
		if len(split) != 2 || split[0] != "subnet" {
			return params.AddSubnetsParams{}, i, errors.New(fmt.Sprintf("%q is not valid SubnetTag", oldSubnet.SubnetTag))
		}
		subnetsParams.Subnets[i] = params.AddSubnetParams{
			CIDR:              split[1],
			SubnetProviderId:  oldSubnet.SubnetProviderId,
			ProviderNetworkId: oldSubnet.ProviderNetworkId,
			SpaceTag:          oldSubnet.SpaceTag,
			VLANTag:           oldSubnet.VLANTag,
			Zones:             oldSubnet.Zones,
		}
	}
	return subnetsParams, -1, nil
}
