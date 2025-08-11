// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnets

import (
	"context"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v6"

	commonnetwork "github.com/juju/juju/apiserver/common/network"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/credential"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/rpc/params"
)

// NetworkService is the interface that is used to interact with the
// network spaces/subnets.
type NetworkService interface {
	// GetAllSpaces returns all spaces for the model.
	GetAllSpaces(ctx context.Context) (network.SpaceInfos, error)
	// GetAllSubnets returns all the subnets for the model.
	GetAllSubnets(ctx context.Context) (network.SubnetInfos, error)
	// SubnetsByCIDR returns the subnets matching the input CIDRs.
	SubnetsByCIDR(ctx context.Context, cidrs ...string) ([]network.SubnetInfo, error)
	// GetProviderAvailabilityZones returns all the availability zones
	// retrieved from the model's cloud provider.
	GetProviderAvailabilityZones(ctx context.Context) (network.AvailabilityZones, error)
}

// CloudService provides access to clouds.
type CloudService interface {
	// Cloud returns the named cloud.
	Cloud(ctx context.Context, name string) (*cloud.Cloud, error)
}

// CredentialService provides access to credentials.
type CredentialService interface {
	// CloudCredential returns the cloud credential for the given tag.
	CloudCredential(ctx context.Context, key credential.Key) (cloud.Credential, error)
}

// ModelConfigService is an interface that provides access to the
// model configuration.
type ModelConfigService interface {
	// ModelConfig returns the current config for the model.
	ModelConfig(ctx context.Context) (*config.Config, error)
}

// API provides the subnets API facade for version 5.
type API struct {
	modelTag names.ModelTag

	authorizer     facade.Authorizer
	logger         corelogger.Logger
	networkService NetworkService
}

func (api *API) checkCanRead(ctx context.Context) error {
	return api.authorizer.HasPermission(ctx, permission.ReadAccess, api.modelTag)
}

// newAPIWithBacking creates a new server-side Subnets API facade with
// a common.NetworkBacking
func newAPIWithBacking(
	modelTag names.ModelTag,
	authorizer facade.Authorizer,
	logger corelogger.Logger,
	networkService NetworkService,
) *API {
	return &API{
		modelTag:       modelTag,
		authorizer:     authorizer,
		logger:         logger,
		networkService: networkService,
	}
}

// AllZones returns all availability zones known to Juju. If a
// zone is unusable, unavailable, or deprecated the Available
// field will be false.
func (api *API) AllZones(ctx context.Context) (params.ZoneResults, error) {
	if err := api.checkCanRead(ctx); err != nil {
		return params.ZoneResults{}, err
	}

	var results params.ZoneResults
	zones, err := api.networkService.GetProviderAvailabilityZones(ctx)
	if err != nil {
		return results, errors.Trace(err)
	}

	results.Results = make([]params.ZoneResult, len(zones))
	for i, zone := range zones {
		results.Results[i].Name = zone.Name()
		results.Results[i].Available = zone.Available()
	}
	return results, nil
}

// ListSubnets returns the matching subnets after applying
// optional filters.
func (api *API) ListSubnets(ctx context.Context, args params.SubnetsFilters) (results params.ListSubnetsResults, err error) {
	if err := api.checkCanRead(ctx); err != nil {
		return params.ListSubnetsResults{}, err
	}

	allSubnets, err := api.networkService.GetAllSubnets(ctx)
	if err != nil {
		return results, errors.Trace(err)
	}

	var spaceFilter network.SpaceName
	if args.SpaceTag != "" {
		tag, err := names.ParseSpaceTag(args.SpaceTag)
		if err != nil {
			return results, errors.Trace(err)
		}
		spaceFilter = network.SpaceName(tag.Id())
	}
	zoneFilter := args.Zone

	for _, subnet := range allSubnets {
		if spaceFilter != "" && subnet.SpaceName != spaceFilter {
			api.logger.Tracef(ctx,
				"filtering subnet %q from space %q not matching filter %q",
				subnet.CIDR, subnet.SpaceName, spaceFilter,
			)
			continue
		}
		zoneSet := set.NewStrings(subnet.AvailabilityZones...)
		if zoneFilter != "" && !zoneSet.IsEmpty() && !zoneSet.Contains(zoneFilter) {
			api.logger.Tracef(ctx,
				"filtering subnet %q with zones %v not matching filter %q",
				subnet.CIDR, subnet.AvailabilityZones, zoneFilter,
			)
			continue
		}

		results.Results = append(results.Results, commonnetwork.SubnetInfoToParamsSubnet(subnet))
	}
	return results, nil
}

// SubnetsByCIDR returns the collection of subnets matching each CIDR in the input.
func (api *API) SubnetsByCIDR(ctx context.Context, arg params.CIDRParams) (params.SubnetsResults, error) {
	result := params.SubnetsResults{}

	if err := api.checkCanRead(ctx); err != nil {
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
			subnetResults[j] = subnetInfoToParamsSubnetWithID(subnet)
		}
		results[i].Subnets = subnetResults
	}
	result.Results = results
	return result, nil
}

// subnetInfoToParamsSubnetWithID converts a network backing subnet to the new
// version of the subnet API parameter.
func subnetInfoToParamsSubnetWithID(subnet network.SubnetInfo) params.SubnetV2 {
	return params.SubnetV2{
		ID:     subnet.ID.String(),
		Subnet: commonnetwork.SubnetInfoToParamsSubnet(subnet),
	}
}
