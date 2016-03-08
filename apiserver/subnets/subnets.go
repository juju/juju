// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnets

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/networkingcommon"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.subnets")

func init() {
	common.RegisterStandardFacade("Subnets", 2, NewAPI)
}

// SubnetsAPI defines the methods the Subnets API facade implements.
type SubnetsAPI interface {
	// AllZones returns all availability zones known to Juju. If a
	// zone is unusable, unavailable, or deprecated the Available
	// field will be false.
	AllZones() (params.ZoneResults, error)

	// AllSpaces returns the tags of all network spaces known to Juju.
	AllSpaces() (params.SpaceResults, error)

	// AddSubnets adds existing subnets to Juju.
	AddSubnets(args params.AddSubnetsParams) (params.ErrorResults, error)

	// ListSubnets returns the matching subnets after applying
	// optional filters.
	ListSubnets(args params.SubnetsFilters) (params.ListSubnetsResults, error)
}

// subnetsAPI implements the SubnetsAPI interface.
type subnetsAPI struct {
	backing    networkingcommon.NetworkBacking
	resources  *common.Resources
	authorizer common.Authorizer
}

// NewAPI creates a new Subnets API server-side facade with a
// state.State backing.
func NewAPI(st *state.State, res *common.Resources, auth common.Authorizer) (SubnetsAPI, error) {
	return newAPIWithBacking(networkingcommon.NewStateShim(st), res, auth)
}

// newAPIWithBacking creates a new server-side Subnets API facade with
// a common.NetworkBacking
func newAPIWithBacking(backing networkingcommon.NetworkBacking, resources *common.Resources, authorizer common.Authorizer) (SubnetsAPI, error) {
	// Only clients can access the Subnets facade.
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}
	return &subnetsAPI{
		backing:    backing,
		resources:  resources,
		authorizer: authorizer,
	}, nil
}

// AllZones is defined on the API interface.
func (api *subnetsAPI) AllZones() (params.ZoneResults, error) {
	return networkingcommon.AllZones(api.backing)
}

// AllSpaces is defined on the API interface.
func (api *subnetsAPI) AllSpaces() (params.SpaceResults, error) {
	var results params.SpaceResults

	spaces, err := api.backing.AllSpaces()
	if err != nil {
		return results, errors.Trace(err)
	}

	results.Results = make([]params.SpaceResult, len(spaces))
	for i, space := range spaces {
		// TODO(dimitern): Add a Tag() a method and use it here. Too
		// early to do it now as it will just complicate the tests.
		tag := names.NewSpaceTag(space.Name())
		results.Results[i].Tag = tag.String()
	}
	return results, nil
}

// AddSubnets is defined on the API interface.
func (api *subnetsAPI) AddSubnets(args params.AddSubnetsParams) (params.ErrorResults, error) {
	return networkingcommon.AddSubnets(api.backing, args)
}

// ListSubnets lists all the available subnets or only those matching
// all given optional filters.
func (api *subnetsAPI) ListSubnets(args params.SubnetsFilters) (results params.ListSubnetsResults, err error) {
	return networkingcommon.ListSubnets(api.backing, args)
}
