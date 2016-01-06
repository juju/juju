// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package discoverspaces

import (
	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/networkingcommon"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

func init() {
	common.RegisterStandardFacade("DiscoverSpaces", 1, NewDiscoverSpacesAPI)
}

// DiscoverSpacesAPI implements the API used by the discoverspaces worker.
type DiscoverSpacesAPI struct {
	st         networkingcommon.NetworkBacking
	resources  *common.Resources
	authorizer common.Authorizer
}

// NewDiscoverSpacesAPI creates a new instance of the DiscoverSpaces API.
func NewDiscoverSpacesAPI(st *state.State, resources *common.Resources, authorizer common.Authorizer) (*DiscoverSpacesAPI, error) {
	return NewDiscoverSpacesAPIWithBacking(networkingcommon.NewStateShim(st), resources, authorizer)
}

func NewDiscoverSpacesAPIWithBacking(st networkingcommon.NetworkBacking, resources *common.Resources, authorizer common.Authorizer) (*DiscoverSpacesAPI, error) {
	if !authorizer.AuthEnvironManager() {
		return nil, common.ErrPerm
	}
	return &DiscoverSpacesAPI{
		st:         st,
		authorizer: authorizer,
		resources:  resources,
	}, nil
}

// EnvironConfig returns the current environment's configuration.
func (api *DiscoverSpacesAPI) EnvironConfig() (params.EnvironConfigResult, error) {
	result := params.EnvironConfigResult{}

	config, err := api.st.EnvironConfig()
	if err != nil {
		return result, err
	}
	allAttrs := config.AllAttrs()
	// No need to obscure any secrets as caller needs to be an
	// EnvironManager to call any api methods.
	result.Config = allAttrs
	return result, nil
}

// CreateSpaces creates a new Juju network space, associating the
// specified subnets with it (optional; can be empty).
func (api *DiscoverSpacesAPI) CreateSpaces(args params.CreateSpacesParams) (results params.ErrorResults, err error) {
	return networkingcommon.CreateSpaces(api.st, args)
}

// ListSpaces lists all the available spaces and their associated subnets.
func (api *DiscoverSpacesAPI) ListSpaces() (results params.DiscoverSpacesResults, err error) {
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

// AddSubnets is defined on the API interface.
func (api *DiscoverSpacesAPI) AddSubnets(args params.AddSubnetsParams) (params.ErrorResults, error) {
	return networkingcommon.AddSubnets(api.st, args)
}

// ListSubnets lists all the available subnets or only those matching
// all given optional filters.
func (api *DiscoverSpacesAPI) ListSubnets(args params.SubnetsFilters) (results params.ListSubnetsResults, err error) {
	return networkingcommon.ListSubnets(api.st, args)
}
