// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"context"
	stdcontext "context"
	"fmt"

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

// SpaceService is the interface that is used to interact with the
// network spaces.
type SpaceService interface {
	AddSpace(ctx context.Context, name string, providerID network.Id, subnetIDs []string) (network.Id, error)
	Space(ctx context.Context, uuid string) (*network.SpaceInfo, error)
	SpaceByName(ctx context.Context, name string) (*network.SpaceInfo, error)
	GetAllSpaces(ctx context.Context) (network.SpaceInfos, error)
	UpdateSpace(ctx context.Context, uuid string, name string) error
	Remove(ctx context.Context, uuid string) error
}

// SubnetService is the interface that is used to interact with the
// network subnets.
type SubnetService interface {
	GetAllSubnets(ctx context.Context) (network.SubnetInfos, error)
	SubnetsByCIDR(ctx context.Context, cidrs ...string) ([]network.SubnetInfo, error)
	Subnet(ctx context.Context, uuid string) (*network.SubnetInfo, error)
	UpdateSubnet(ctx context.Context, uuid, spaceUUID string) error
}

// API provides the spaces API facade for version 6.
type API struct {
	reloadSpacesAPI ReloadSpaces

	spaceService                SpaceService
	subnetService               SubnetService
	backing                     Backing
	resources                   facade.Resources
	auth                        facade.Authorizer
	credentialInvalidatorGetter envcontext.ModelCredentialInvalidatorGetter

	check     BlockChecker
	opFactory OpFactory
	logger    loggo.Logger
}

type apiConfig struct {
	ReloadSpacesAPI             ReloadSpaces
	Backing                     Backing
	Check                       BlockChecker
	CredentialInvalidatorGetter envcontext.ModelCredentialInvalidatorGetter
	Resources                   facade.Resources
	Authorizer                  facade.Authorizer
	Factory                     OpFactory
	logger                      loggo.Logger
	SpaceService                SpaceService
	SubnetService               SubnetService
}

// newAPIWithBacking creates a new server-side Spaces API facade with
// the given Backing.
func newAPIWithBacking(cfg apiConfig) (*API, error) {
	// Only clients can access the Spaces facade.
	if !cfg.Authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	return &API{
		reloadSpacesAPI:             cfg.ReloadSpacesAPI,
		backing:                     cfg.Backing,
		resources:                   cfg.Resources,
		auth:                        cfg.Authorizer,
		credentialInvalidatorGetter: cfg.CredentialInvalidatorGetter,
		check:                       cfg.Check,
		opFactory:                   cfg.Factory,
		logger:                      cfg.logger,
		spaceService:                cfg.SpaceService,
		subnetService:               cfg.SubnetService,
	}, nil
}

// CreateSpaces creates a new Juju network space, associating the
// specified subnets with it (optional; can be empty).
func (api *API) CreateSpaces(ctx stdcontext.Context, args params.CreateSpacesParams) (results params.ErrorResults, err error) {
	err = api.auth.HasPermission(permission.AdminAccess, api.backing.ModelTag())
	if err != nil {
		return results, err
	}
	if err := api.check.ChangeAllowed(ctx); err != nil {
		return results, errors.Trace(err)
	}
	if err = api.checkSupportsSpaces(ctx); err != nil {
		return results, apiservererrors.ServerError(errors.Trace(err))
	}

	results.Results = make([]params.ErrorResult, len(args.Spaces))

	for i, space := range args.Spaces {
		err := api.createOneSpace(ctx, space)
		if err == nil {
			continue
		}
		results.Results[i].Error = apiservererrors.ServerError(errors.Trace(err))
	}

	return results, nil
}

// createOneSpace creates one new Juju network space, associating the
// specified subnets with it (optional; can be empty).
func (api *API) createOneSpace(ctx context.Context, args params.CreateSpaceParams) error {
	// Validate the args, assemble information for api.backing.AddSpaces
	spaceTag, err := names.ParseSpaceTag(args.SpaceTag)
	if err != nil {
		return errors.Trace(err)
	}

	for _, cidr := range args.CIDRs {
		if !network.IsValidCIDR(cidr) {
			return errors.New(fmt.Sprintf("%q is not a valid CIDR", cidr))
		}
	}

	subnets, err := api.subnetService.SubnetsByCIDR(ctx, args.CIDRs...)
	if err != nil {
		return err
	}
	subnetIDs := make([]string, len(args.CIDRs))
	for i, subnet := range subnets {
		subnetIDs[i] = subnet.ID.String()
	}

	// Add the validated space.
	_, err = api.spaceService.AddSpace(ctx, spaceTag.Id(), network.Id(args.ProviderId), subnetIDs)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// ListSpaces lists all the available spaces and their associated subnets.
func (api *API) ListSpaces(ctx stdcontext.Context) (results params.ListSpacesResults, err error) {
	err = api.auth.HasPermission(permission.ReadAccess, api.backing.ModelTag())
	if err != nil {
		return results, err
	}

	err = api.checkSupportsSpaces(ctx)
	if err != nil {
		return results, apiservererrors.ServerError(errors.Trace(err))
	}

	spaces, err := api.spaceService.GetAllSpaces(ctx)
	if err != nil {
		return results, errors.Trace(err)
	}

	results.Results = make([]params.Space, len(spaces))
	for i, space := range spaces {
		result := params.Space{}
		result.Id = space.ID
		result.Name = string(space.Name)

		if err != nil {
			err = errors.Annotatef(err, "fetching spaces")
			result.Error = apiservererrors.ServerError(err)
			results.Results[i] = result
			continue
		}
		subnets := space.Subnets

		result.Subnets = make([]params.Subnet, len(subnets))
		for i, subnet := range subnets {
			result.Subnets[i] = networkingcommon.SubnetInfoToParamsSubnet(subnet)
		}
		results.Results[i] = result
	}
	return results, nil
}

// ShowSpace shows the spaces for a set of given entities.
func (api *API) ShowSpace(ctx stdcontext.Context, entities params.Entities) (params.ShowSpaceResults, error) {
	err := api.auth.HasPermission(permission.ReadAccess, api.backing.ModelTag())
	if err != nil {
		return params.ShowSpaceResults{}, err
	}

	err = api.checkSupportsSpaces(ctx)
	if err != nil {
		return params.ShowSpaceResults{}, apiservererrors.ServerError(errors.Trace(err))
	}

	// Retrieve the list of all spaces, needed for the bindings.
	allSpaces, err := api.spaceService.GetAllSpaces(ctx)
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
		space, err := api.spaceService.SpaceByName(ctx, spaceName.Id())
		if err != nil {
			newErr := errors.Annotatef(err, "fetching space %q", spaceName)
			results[i].Error = apiservererrors.ServerError(newErr)
			continue
		}
		result.Space.Name = string(space.Name)
		result.Space.Id = space.ID
		subnets := space.Subnets

		result.Space.Subnets = make([]params.Subnet, len(subnets))
		for i, subnet := range subnets {
			result.Space.Subnets[i] = networkingcommon.SubnetInfoToParamsSubnet(subnet)
		}

		applications, err := api.applicationsBoundToSpace(space.ID, allSpaces)
		if err != nil {
			newErr := errors.Annotatef(err, "fetching applications")
			results[i].Error = apiservererrors.ServerError(newErr)
			continue
		}
		result.Applications = applications

		machineCount, err := api.getMachineCountBySpaceID(space.ID)
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
func (api *API) ReloadSpaces(ctx stdcontext.Context) error {
	return api.reloadSpacesAPI.ReloadSpaces(ctx)
}

// checkSupportsSpaces checks if the environment implements NetworkingEnviron
// and also if it supports spaces.
func (api *API) checkSupportsSpaces(ctx stdcontext.Context) error {
	env, err := environs.GetEnviron(ctx, api.backing, environs.New)
	if err != nil {
		return errors.Annotate(err, "getting environ")
	}
	invalidatorFunc, err := api.credentialInvalidatorGetter()
	if err != nil {
		return errors.Trace(err)
	}
	callCtx := envcontext.WithCredentialInvalidator(ctx, invalidatorFunc)
	if !environs.SupportsSpaces(callCtx, env) {
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

func (api *API) applicationsBoundToSpace(spaceID string, allSpaces network.SpaceInfos) ([]string, error) {
	allBindings, err := api.backing.AllEndpointBindings(allSpaces)
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
func (api *API) ensureSpacesAreMutable(ctx stdcontext.Context) error {
	err := api.auth.HasPermission(permission.AdminAccess, api.backing.ModelTag())
	if err != nil {
		return err
	}
	if err := api.check.ChangeAllowed(ctx); err != nil {
		return errors.Trace(err)
	}
	if err = api.ensureSpacesNotProviderSourced(ctx); err != nil {
		return apiservererrors.ServerError(errors.Trace(err))
	}
	return nil
}

// ensureSpacesNotProviderSourced checks if the environment implements
// NetworkingEnviron and also if it supports provider spaces.
// An error is returned if it is the provider and not the Juju operator
// that determines the space topology.
func (api *API) ensureSpacesNotProviderSourced(ctx stdcontext.Context) error {
	env, err := environs.GetEnviron(ctx, api.backing, environs.New)
	if err != nil {
		return errors.Annotate(err, "retrieving environ")
	}

	netEnv, ok := env.(environs.NetworkingEnviron)
	if !ok {
		return errors.NotSupportedf("provider networking")
	}

	invalidatorFunc, err := api.credentialInvalidatorGetter()
	if err != nil {
		return errors.Trace(err)
	}
	callCtx := envcontext.WithCredentialInvalidator(ctx, invalidatorFunc)
	providerSourced, err := netEnv.SupportsSpaceDiscovery(callCtx)
	if err != nil {
		return errors.Trace(err)
	}

	if providerSourced {
		return errors.NotSupportedf("modifying provider-sourced spaces")
	}
	return nil
}
