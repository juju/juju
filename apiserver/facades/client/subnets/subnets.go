// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnets

import (
	stdcontext "context"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"

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

// API provides the subnets API facade for version 5.
type API struct {
	backing                     Backing
	resources                   facade.Resources
	authorizer                  facade.Authorizer
	credentialInvalidatorGetter envcontext.ModelCredentialInvalidatorGetter
	logger                      loggo.Logger
}

func (api *API) checkCanRead() error {
	return api.authorizer.HasPermission(permission.ReadAccess, api.backing.ModelTag())
}

// newAPIWithBacking creates a new server-side Subnets API facade with
// a common.NetworkBacking
func newAPIWithBacking(
	backing Backing, credentialInvalidatorGetter envcontext.ModelCredentialInvalidatorGetter,
	resources facade.Resources, authorizer facade.Authorizer, logger loggo.Logger,
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
			api.logger.Tracef(
				"filtering subnet %q from space %q not matching filter %q",
				subnet.CIDR(), subnet.SpaceName(), spaceFilter,
			)
			continue
		}
		zoneSet := set.NewStrings(subnet.AvailabilityZones()...)
		if zoneFilter != "" && !zoneSet.IsEmpty() && !zoneSet.Contains(zoneFilter) {
			api.logger.Tracef(
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

		subnets, err := api.backing.SubnetsByCIDR(cidr)
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
