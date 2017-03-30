// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package discoverspaces

import (
	"github.com/juju/errors"
	"github.com/juju/utils/set"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/networkingcommon"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
)

// API implements the API used by the discoverspaces worker.
type API struct {
	st         networkingcommon.NetworkBacking
	resources  facade.Resources
	authorizer facade.Authorizer
}

// NewAPI creates a new instance of the DiscoverSpaces API.
func NewAPI(st *state.State, resources facade.Resources, authorizer facade.Authorizer) (*API, error) {
	return NewAPIWithBacking(networkingcommon.NewStateShim(st), resources, authorizer)
}

// NewAPIWithBacking creates an API instance from the given network
// backing (primarily useful from tests).
func NewAPIWithBacking(st networkingcommon.NetworkBacking, resources facade.Resources, authorizer facade.Authorizer) (*API, error) {
	if !authorizer.AuthController() {
		return nil, common.ErrPerm
	}
	return &API{
		st:         st,
		authorizer: authorizer,
		resources:  resources,
	}, nil
}

// ModelConfig returns the current model's configuration.
func (api *API) ModelConfig() (params.ModelConfigResult, error) {
	result := params.ModelConfigResult{}

	config, err := api.st.ModelConfig()
	if err != nil {
		return result, err
	}
	allAttrs := config.AllAttrs()
	// No need to obscure any secrets as caller needs to be a ModelManager to
	// call any api methods.
	result.Config = allAttrs
	return result, nil
}

// CreateSpaces creates a new Juju network space, associating the
// specified subnets with it (optional; can be empty).
func (api *API) CreateSpaces(args params.CreateSpacesParams) (results params.ErrorResults, err error) {
	return networkingcommon.CreateSpaces(api.st, args)
}

// ListSpaces lists all the available spaces and their associated subnets.
func (api *API) ListSpaces() (results params.DiscoverSpacesResults, err error) {
	spaces, err := api.st.AllSpaces()
	if err != nil {
		return results, errors.Trace(err)
	}

	results.Results = make([]params.ProviderSpace, len(spaces))
	for i, space := range spaces {
		result := params.ProviderSpace{}
		result.ProviderId = string(space.ProviderId())
		result.Name = space.Name()

		subnets, err := space.Subnets()
		if err != nil {
			err = errors.Annotatef(err, "fetching subnets")
			result.Error = common.ServerError(err)
			results.Results[i] = result
			continue
		}

		result.Subnets = make([]params.Subnet, len(subnets))
		for i, subnet := range subnets {
			result.Subnets[i] = networkingcommon.BackingSubnetToParamsSubnet(subnet)
		}
		results.Results[i] = result
	}
	return results, nil
}

// AddSubnets adds the passed subnet info to the backing store.
func (api *API) AddSubnets(args params.AddSubnetsParams) (params.ErrorResults, error) {
	var empty params.ErrorResults
	if len(args.Subnets) == 0 {
		return empty, nil
	}
	spaces, err := api.st.AllSpaces()
	if err != nil {
		return empty, errors.Trace(err)
	}
	spaceNames := set.NewStrings()
	for _, space := range spaces {
		spaceNames.Add(space.Name())
	}

	addOneSubnet := func(arg params.AddSubnetParams) error {
		if arg.SubnetProviderId == "" {
			return errors.Trace(errors.New("SubnetProviderId is required"))
		}
		spaceName := ""
		if arg.SpaceTag != "" {
			spaceTag, err := names.ParseSpaceTag(arg.SpaceTag)
			if err != nil {
				return errors.Annotate(err, "SpaceTag is invalid")
			}
			spaceName = spaceTag.Id()
			if !spaceNames.Contains(spaceName) {
				return errors.NotFoundf("space %q", spaceName)
			}
		}
		if arg.SubnetTag == "" {
			return errors.New("SubnetTag is required")
		}
		subnetTag, err := names.ParseSubnetTag(arg.SubnetTag)
		if err != nil {
			return errors.Annotate(err, "SubnetTag is invalid")
		}
		_, err = api.st.AddSubnet(networkingcommon.BackingSubnetInfo{
			ProviderId:        network.Id(arg.SubnetProviderId),
			ProviderNetworkId: network.Id(arg.ProviderNetworkId),
			CIDR:              subnetTag.Id(),
			VLANTag:           arg.VLANTag,
			AvailabilityZones: arg.Zones,
			SpaceName:         spaceName,
		})
		return errors.Trace(err)
	}

	results := make([]params.ErrorResult, len(args.Subnets))
	for i, arg := range args.Subnets {
		results[i].Error = common.ServerError(addOneSubnet(arg))
	}
	return params.ErrorResults{Results: results}, nil
}

// ListSubnets lists all the available subnets or only those matching
// all given optional filters.
func (api *API) ListSubnets(args params.SubnetsFilters) (results params.ListSubnetsResults, err error) {
	return networkingcommon.ListSubnets(api.st, args)
}
