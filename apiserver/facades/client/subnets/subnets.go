// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnets

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/networkingcommon"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
)

// APIv2 provides the subnets API facade for versions < 3.
type APIv2 struct {
	*API
}

// API provides the subnets API facade for version 3.
type API struct {
	backing    networkingcommon.NetworkBacking
	resources  facade.Resources
	authorizer facade.Authorizer
	context    context.ProviderCallContext
}

// NewAPIv2 is a wrapper that creates a V2 subnets API.
func NewAPIv2(st *state.State, res facade.Resources, auth facade.Authorizer) (*APIv2, error) {
	api, err := NewAPI(st, res, auth)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv2{api}, nil
}

// NewAPI creates a new Subnets API server-side facade with a
// state.State backing.
func NewAPI(st *state.State, res facade.Resources, auth facade.Authorizer) (*API, error) {
	stateshim, err := networkingcommon.NewStateShim(st)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return newAPIWithBacking(stateshim, state.CallContext(st), res, auth)
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
func newAPIWithBacking(backing networkingcommon.NetworkBacking, ctx context.ProviderCallContext, resources facade.Resources, authorizer facade.Authorizer) (*API, error) {
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
	return networkingcommon.AllZones(api.context, api.backing)
}

// AllSpaces returns the tags of all network spaces known to Juju.
func (api *API) AllSpaces() (params.SpaceResults, error) {
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
		// TODO(dimitern): Add a Tag() a method and use it here. Too
		// early to do it now as it will just complicate the tests.
		tag := names.NewSpaceTag(space.Name())
		results.Results[i].Tag = tag.String()
	}
	return results, nil
}

// AddSubnets adds existing subnets to Juju.
func (api *API) AddSubnets(args params.AddSubnetsParams) (params.ErrorResults, error) {
	if err := api.checkCanWrite(); err != nil {
		return params.ErrorResults{}, err
	}
	return networkingcommon.AddSubnets(api.context, api.backing, args)
}

// AddSubnets adds existing subnets to Juju.  Args are converted to
// the new form for compatibility
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
	return networkingcommon.AddSubnets(api.context, api.backing, newArgs)
}

// ListSubnets returns the matching subnets after applying
// optional filters.
func (api *API) ListSubnets(args params.SubnetsFilters) (results params.ListSubnetsResults, err error) {
	if err := api.checkCanRead(); err != nil {
		return params.ListSubnetsResults{}, err
	}

	return networkingcommon.ListSubnets(api.backing, args)
}

func convertToAddSubnetsParams(old params.AddSubnetsParamsV2) (params.AddSubnetsParams, int, error) {
	new := params.AddSubnetsParams{
		Subnets: make([]params.AddSubnetParams, len(old.Subnets)),
	}
	for i, oldSubnet := range old.Subnets {
		split := strings.Split(oldSubnet.SubnetTag, "-")
		if len(split) != 2 || split[0] != "subnet" {
			return params.AddSubnetsParams{}, i, errors.New(fmt.Sprintf("%q is not valid SubnetTag", oldSubnet.SubnetTag))
		}
		new.Subnets[i] = params.AddSubnetParams{
			CIDR:              split[1],
			SubnetProviderId:  oldSubnet.SubnetProviderId,
			ProviderNetworkId: oldSubnet.ProviderNetworkId,
			SpaceTag:          oldSubnet.SpaceTag,
			VLANTag:           oldSubnet.VLANTag,
			Zones:             oldSubnet.Zones,
		}
	}
	return new, -1, nil
}
