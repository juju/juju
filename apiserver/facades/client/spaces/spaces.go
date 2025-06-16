// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"fmt"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/common/networkingcommon"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/rpc/params"
)

var logger = loggo.GetLogger("juju.apiserver.spaces")

// API provides the spaces API facade for version 6.
type API struct {
	reloadSpacesAPI ReloadSpaces

	backing   Backing
	resources facade.Resources
	auth      facade.Authorizer
	context   context.ProviderCallContext

	check     BlockChecker
	opFactory OpFactory
}

type apiConfig struct {
	ReloadSpacesAPI ReloadSpaces
	Backing         Backing
	Check           BlockChecker
	Context         context.ProviderCallContext
	Resources       facade.Resources
	Authorizer      facade.Authorizer
	Factory         OpFactory
}

// newAPIWithBacking creates a new server-side Spaces API facade with
// the given Backing.
func newAPIWithBacking(cfg apiConfig) (*API, error) {
	// Only clients can access the Spaces facade.
	if !cfg.Authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	return &API{
		reloadSpacesAPI: cfg.ReloadSpacesAPI,
		backing:         cfg.Backing,
		resources:       cfg.Resources,
		auth:            cfg.Authorizer,
		context:         cfg.Context,
		check:           cfg.Check,
		opFactory:       cfg.Factory,
	}, nil
}

// CreateSpaces creates a new Juju network space, associating the
// specified subnets with it (optional; can be empty).
func (api *API) CreateSpaces(args params.CreateSpacesParams) (results params.ErrorResults, err error) {
	err = api.auth.HasPermission(permission.AdminAccess, api.backing.ModelTag())
	if err != nil {
		return results, err
	}
	if err := api.check.ChangeAllowed(); err != nil {
		return results, errors.Trace(err)
	}
	if err = api.checkSupportsSpaces(); err != nil {
		return results, apiservererrors.ServerError(errors.Trace(err))
	}

	results.Results = make([]params.ErrorResult, len(args.Spaces))

	for i, space := range args.Spaces {
		err := api.createOneSpace(space)
		if err == nil {
			continue
		}
		results.Results[i].Error = apiservererrors.ServerError(errors.Trace(err))
	}

	return results, nil
}

// createOneSpace creates one new Juju network space, associating the
// specified subnets with it (optional; can be empty).
func (api *API) createOneSpace(args params.CreateSpaceParams) error {
	// Validate the args, assemble information for api.backing.AddSpaces
	spaceTag, err := names.ParseSpaceTag(args.SpaceTag)
	if err != nil {
		return errors.Trace(err)
	}

	subnetIDs := make([]string, len(args.CIDRs))
	for i, cidr := range args.CIDRs {
		if !network.IsValidCIDR(cidr) {
			return errors.New(fmt.Sprintf("%q is not a valid CIDR", cidr))
		}
		subnet, err := api.backing.SubnetByCIDR(cidr)
		if err != nil {
			return err
		}
		subnetIDs[i] = subnet.ID()
	}

	// Add the validated space.
	_, err = api.backing.AddSpace(spaceTag.Id(), network.Id(args.ProviderId), subnetIDs, args.Public)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// ListSpaces lists all the available spaces and their associated subnets.
func (api *API) ListSpaces() (results params.ListSpacesResults, err error) {
	err = api.auth.HasPermission(permission.ReadAccess, api.backing.ModelTag())
	if err != nil {
		return results, err
	}

	err = api.checkSupportsSpaces()
	if err != nil {
		return results, apiservererrors.ServerError(errors.Trace(err))
	}

	spaces, err := api.backing.AllSpaces()
	if err != nil {
		return results, errors.Trace(err)
	}

	results.Results = make([]params.Space, len(spaces))
	for i, space := range spaces {
		result := params.Space{}
		result.Id = space.Id()
		result.Name = space.Name()

		spaceInfo, err := space.NetworkSpace()
		if err != nil {
			err = errors.Annotatef(err, "fetching subnets")
			result.Error = apiservererrors.ServerError(err)
			results.Results[i] = result
			continue
		}
		subnets := spaceInfo.Subnets

		result.Subnets = make([]params.Subnet, len(subnets))
		for i, subnet := range subnets {
			result.Subnets[i] = networkingcommon.SubnetInfoToParamsSubnet(subnet)
		}
		results.Results[i] = result
	}
	return results, nil
}

// ShowSpace shows the spaces for a set of given entities.
func (api *API) ShowSpace(entities params.Entities) (params.ShowSpaceResults, error) {
	err := api.auth.HasPermission(permission.ReadAccess, api.backing.ModelTag())
	if err != nil {
		return params.ShowSpaceResults{}, err
	}

	err = api.checkSupportsSpaces()
	if err != nil {
		return params.ShowSpaceResults{}, apiservererrors.ServerError(errors.Trace(err))
	}
	results := make([]params.ShowSpaceResult, len(entities.Entities))
	for i, entity := range entities.Entities {
		spaceName, err := names.ParseSpaceTag(entity.Tag)
		if err != nil {
			results[i].Error = apiservererrors.ServerError(errors.Trace(err))
			continue
		}
		var result params.ShowSpaceResult
		space, err := api.backing.SpaceByName(spaceName.Id())
		if err != nil {
			newErr := errors.Annotatef(err, "fetching space %q", spaceName)
			results[i].Error = apiservererrors.ServerError(newErr)
			continue
		}
		result.Space.Name = space.Name()
		result.Space.Id = space.Id()
		spaceInfo, err := space.NetworkSpace()
		if err != nil {
			newErr := errors.Annotatef(err, "fetching subnets")
			results[i].Error = apiservererrors.ServerError(newErr)
			continue
		}
		subnets := spaceInfo.Subnets

		result.Space.Subnets = make([]params.Subnet, len(subnets))
		for i, subnet := range subnets {
			result.Space.Subnets[i] = networkingcommon.SubnetInfoToParamsSubnet(subnet)
		}

		applications, err := api.applicationsBoundToSpace(space.Id())
		if err != nil {
			newErr := errors.Annotatef(err, "fetching applications")
			results[i].Error = apiservererrors.ServerError(newErr)
			continue
		}
		result.Applications = applications

		machineCount, err := api.getMachineCountBySpaceID(space.Id())
		if err != nil {
			newErr := errors.Annotatef(err, "fetching machine count")
			results[i].Error = apiservererrors.ServerError(newErr)
			continue
		}

		result.MachineCount = machineCount
		results[i] = result
	}

	return params.ShowSpaceResults{Results: results}, err
}

// ReloadSpaces refreshes spaces from substrate
func (api *API) ReloadSpaces() error {
	return api.reloadSpacesAPI.ReloadSpaces()
}

// checkSupportsSpaces checks if the environment implements NetworkingEnviron
// and also if it supports spaces.
func (api *API) checkSupportsSpaces() error {
	env, err := environs.GetEnviron(api.backing, environs.New)
	if err != nil {
		return errors.Annotate(err, "getting environ")
	}
	if !environs.SupportsSpaces(api.context, env) {
		return errors.NotSupportedf("spaces")
	}
	return nil
}

func (api *API) getMachineCountBySpaceID(spaceID string) (int, error) {
	var count int
	machines, err := api.backing.AllMachines()
	if err != nil {
		return 0, errors.Trace(err)
	}
	for _, machine := range machines {
		spacesSet, err := machine.AllSpaces()
		if err != nil {
			return 0, errors.Trace(err)
		}
		if spacesSet.Contains(spaceID) {
			count++
		}
	}
	return count, nil
}

func (api *API) applicationsBoundToSpace(spaceID string) ([]string, error) {
	allBindings, err := api.backing.AllEndpointBindings()
	if err != nil {
		return nil, errors.Trace(err)
	}

	applications := set.NewStrings()
	for app, bindings := range allBindings {
		for _, boundSpace := range bindings.Map() {
			if boundSpace == spaceID {
				applications.Add(app)
				break
			}
		}
	}
	return applications.SortedValues(), nil
}

// ensureSpacesAreMutable checks that the current user
// is allowed to edit the Space topology.
func (api *API) ensureSpacesAreMutable() error {
	err := api.auth.HasPermission(permission.AdminAccess, api.backing.ModelTag())
	if err != nil {
		return err
	}
	if err := api.check.ChangeAllowed(); err != nil {
		return errors.Trace(err)
	}
	if err = api.ensureSpacesNotProviderSourced(); err != nil {
		return apiservererrors.ServerError(errors.Trace(err))
	}
	return nil
}

// ensureSpacesNotProviderSourced checks if the environment implements
// NetworkingEnviron and also if it supports provider spaces.
// An error is returned if it is the provider and not the Juju operator
// that determines the space topology.
func (api *API) ensureSpacesNotProviderSourced() error {
	env, err := environs.GetEnviron(api.backing, environs.New)
	if err != nil {
		return errors.Annotate(err, "retrieving environ")
	}

	netEnv, ok := env.(environs.NetworkingEnviron)
	if !ok {
		return errors.NotSupportedf("provider networking")
	}

	providerSourced, err := netEnv.SupportsSpaceDiscovery(api.context)
	if err != nil {
		return errors.Trace(err)
	}

	if providerSourced {
		return errors.NotSupportedf("modifying provider-sourced spaces")
	}
	return nil
}
