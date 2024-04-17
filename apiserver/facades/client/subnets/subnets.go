// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnets

import (
	"context"
	stdcontext "context"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/common/networkingcommon"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/rpc/params"
)

// Backing contains the state methods used in this package.
type Backing interface {
	environs.EnvironConfigGetter

	// AvailabilityZones returns all cached availability zones (i.e.
	// not from the provider, but in state).
	AvailabilityZones() (network.AvailabilityZones, error)

	// SetAvailabilityZones replaces the cached list of availability
	// zones with the given zones.
	SetAvailabilityZones(network.AvailabilityZones) error

	// ModelTag returns the tag of the model this state is associated to.
	ModelTag() names.ModelTag
}

// NetworkService is the interface that is used to interact with the
// network spaces/subnets.
type NetworkService interface {
	// Space returns a space from state that matches the input ID.
	// An error is returned if the space does not exist or if there was a problem
	// accessing its information.
	Space(ctx context.Context, uuid string) (*network.SpaceInfo, error)
	// SpaceByName returns a space from state that matches the input name.
	// An error is returned that satisfied errors.NotFound if the space was not found
	// or an error static any problems fetching the given space.
	SpaceByName(ctx context.Context, name string) (*network.SpaceInfo, error)
	// GetAllSpaces returns all spaces for the model.
	GetAllSpaces(ctx context.Context) (network.SpaceInfos, error)
	// GetAllSubnets returns all the subnets for the model.
	GetAllSubnets(ctx context.Context) (network.SubnetInfos, error)
	// SubnetsByCIDR returns the subnets matching the input CIDRs.
	SubnetsByCIDR(ctx context.Context, cidrs ...string) ([]network.SubnetInfo, error)
}

// API provides the subnets API facade for version 5.
type API struct {
	backing                     Backing
	resources                   facade.Resources
	authorizer                  facade.Authorizer
	credentialInvalidatorGetter envcontext.ModelCredentialInvalidatorGetter
	logger                      loggo.Logger
	networkService              NetworkService
}

func (api *API) checkCanRead() error {
	return api.authorizer.HasPermission(permission.ReadAccess, api.backing.ModelTag())
}

// newAPIWithBacking creates a new server-side Subnets API facade with
// a common.NetworkBacking
func newAPIWithBacking(
	backing Backing,
	credentialInvalidatorGetter envcontext.ModelCredentialInvalidatorGetter,
	resources facade.Resources,
	authorizer facade.Authorizer,
	logger loggo.Logger,
	networkService NetworkService,
) (*API, error) {
	// Only clients can access the Subnets facade.
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}
	return &API{
		backing:                     backing,
		resources:                   resources,
		authorizer:                  authorizer,
		credentialInvalidatorGetter: credentialInvalidatorGetter,
		logger:                      logger,
		networkService:              networkService,
	}, nil
}

// AllZones returns all availability zones known to Juju. If a
// zone is unusable, unavailable, or deprecated the Available
// field will be false.
func (api *API) AllZones(ctx stdcontext.Context) (params.ZoneResults, error) {
	if err := api.checkCanRead(); err != nil {
		return params.ZoneResults{}, err
	}
	invalidator, err := api.credentialInvalidatorGetter()
	if err != nil {
		return params.ZoneResults{}, errors.Trace(err)
	}
	callCtx := envcontext.WithCredentialInvalidator(ctx, invalidator)
	return allZones(callCtx, api.backing, api.logger)
}

// ListSubnets returns the matching subnets after applying
// optional filters.
func (api *API) ListSubnets(ctx stdcontext.Context, args params.SubnetsFilters) (results params.ListSubnetsResults, err error) {
	if err := api.checkCanRead(); err != nil {
		return params.ListSubnetsResults{}, err
	}

	allSubnets, err := api.networkService.GetAllSubnets(ctx)
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

	for _, subnet := range allSubnets {
		if spaceFilter != "" && subnet.SpaceName != spaceFilter {
			api.logger.Tracef(
				"filtering subnet %q from space %q not matching filter %q",
				subnet.CIDR, subnet.SpaceName, spaceFilter,
			)
			continue
		}
		zoneSet := set.NewStrings(subnet.AvailabilityZones...)
		if zoneFilter != "" && !zoneSet.IsEmpty() && !zoneSet.Contains(zoneFilter) {
			api.logger.Tracef(
				"filtering subnet %q with zones %v not matching filter %q",
				subnet.CIDR, subnet.AvailabilityZones, zoneFilter,
			)
			continue
		}

		results.Results = append(results.Results, networkingcommon.BackingSubnetToParamsSubnet(subnet))
	}
	return results, nil
}

// SubnetsByCIDR returns the collection of subnets matching each CIDR in the input.
func (api *API) SubnetsByCIDR(ctx stdcontext.Context, arg params.CIDRParams) (params.SubnetsResults, error) {
	result := params.SubnetsResults{}

	if err := api.checkCanRead(); err != nil {
		return result, err
	}

	results := make([]params.SubnetsResult, len(arg.CIDRS))
	for i, cidr := range arg.CIDRS {
		if !network.IsValidCIDR(cidr) {
			results[i].Error = apiservererrors.ServerError(errors.NotValidf("CIDR %q", cidr))
			continue
		}

		// TODO(nvinuesa): the SubnetsByCIDR() method takes a list
		// of CIDRs and will return every subnet included in them. We
		// should therefore refactor this so we don't hit the db on
		// every CIDR. The API response should be revisited.
		subnets, err := api.networkService.SubnetsByCIDR(ctx, cidr)
		if err != nil {
			results[i].Error = apiservererrors.ServerError(err)
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
