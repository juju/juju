// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"context"
	"sort"
	"strings"

	"github.com/juju/clock"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	commonmodel "github.com/juju/juju/apiserver/common/model"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	apiservercharms "github.com/juju/juju/apiserver/internal/charms"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/leadership"
	corelease "github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/life"
	corelogger "github.com/juju/juju/core/logger"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	domainapplication "github.com/juju/juju/domain/application"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	domainlife "github.com/juju/juju/domain/life"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/domain/relation"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	resolveerrors "github.com/juju/juju/domain/resolve/errors"
	"github.com/juju/juju/domain/unitstate"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// UniterAPI implements the latest version (v21) of the Uniter API.
type UniterAPI struct {
	*StatusAPI
	*StorageAPI

	*common.APIAddresser
	*commonmodel.ModelConfigWatcher
	*common.RebootRequester
	*common.UnitStateAPI

	modelUUID model.UUID
	modelType model.ModelType

	lxdProfileAPI           *LXDProfileAPI
	st                      *state.State
	clock                   clock.Clock
	auth                    facade.Authorizer
	resources               facade.Resources
	leadershipChecker       leadership.Checker
	leadershipRevoker       leadership.Revoker
	accessUnit              common.GetAuthFunc
	accessApplication       common.GetAuthFunc
	accessUnitOrApplication common.GetAuthFunc
	accessMachine           common.GetAuthFunc
	containerBrokerFunc     caas.NewContainerBrokerFunc
	watcherRegistry         facade.WatcherRegistry

	applicationService      ApplicationService
	resolveService          ResolveService
	statusService           StatusService
	controllerConfigService ControllerConfigService
	machineService          MachineService
	modelConfigService      ModelConfigService
	modelInfoService        ModelInfoService
	modelProviderService    ModelProviderService
	networkService          NetworkService
	portService             PortService
	relationService         RelationService
	secretService           SecretService
	unitStateService        UnitStateService

	store objectstore.ObjectStore

	// A cloud spec can only be accessed for the model of the unit or
	// application that is authorised for this API facade.
	// We do not need to use an AuthFunc, because we do not need to pass a tag.
	accessCloudSpec func(ctx context.Context) (func() bool, error)

	logger corelogger.Logger
}

type UniterAPIv19 struct {
	*UniterAPIv20
}

type UniterAPIv20 struct {
	*UniterAPI
}

// EnsureDead calls EnsureDead on each given unit from state.
// If it's Alive, nothing will happen.
func (u *UniterAPI) EnsureDead(ctx context.Context, args params.Entities) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	canModify, err := u.accessUnit(ctx)
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			return params.ErrorResults{}, errors.Trace(err)
		}
		if !canModify(tag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}

		unitName, err := coreunit.NewName(tag.Id())
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if err := u.applicationService.EnsureUnitDead(ctx, unitName, u.leadershipRevoker); err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
		}
	}
	return result, nil
}

// OpenedMachinePortRangesByEndpoint returns the port ranges opened by each
// unit on the provided machines grouped by application endpoint.
func (u *UniterAPI) OpenedMachinePortRangesByEndpoint(ctx context.Context, args params.Entities) (params.OpenPortRangesByEndpointResults, error) {
	result := params.OpenPortRangesByEndpointResults{
		Results: make([]params.OpenPortRangesByEndpointResult, len(args.Entities)),
	}
	canAccess, err := u.accessMachine(ctx)
	if err != nil {
		return params.OpenPortRangesByEndpointResults{}, err
	}
	for i, entity := range args.Entities {
		machPortRanges, err := u.getOneMachineOpenedPortRanges(ctx, canAccess, entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		result.Results[i].UnitPortRanges = make(map[string][]params.OpenUnitPortRangesByEndpoint)
		for unitName, groupedPortRanges := range machPortRanges {
			unitTag := names.NewUnitTag(unitName.String()).String()
			for endpointName, portRanges := range groupedPortRanges {
				result.Results[i].UnitPortRanges[unitTag] = append(
					result.Results[i].UnitPortRanges[unitTag],
					params.OpenUnitPortRangesByEndpoint{
						Endpoint:   endpointName,
						PortRanges: transform.Slice(portRanges, params.FromNetworkPortRange),
					},
				)
			}

			// Ensure results are sorted by endpoint name to be consistent.
			sort.Slice(result.Results[i].UnitPortRanges[unitTag], func(a, b int) bool {
				return result.Results[i].UnitPortRanges[unitTag][a].Endpoint < result.Results[i].UnitPortRanges[unitTag][b].Endpoint
			})
		}
	}
	return result, nil
}

func (u *UniterAPI) getOneMachineOpenedPortRanges(ctx context.Context, canAccess common.AuthFunc, machineTag string) (map[coreunit.Name]network.GroupedPortRanges, error) {
	tag, err := names.ParseMachineTag(machineTag)
	if err != nil {
		return nil, apiservererrors.ErrPerm
	}
	if !canAccess(tag) {
		return nil, apiservererrors.ErrPerm
	}
	machineUUID, err := u.machineService.GetMachineUUID(ctx, coremachine.Name(tag.Id()))
	if err != nil {
		return nil, internalerrors.Errorf("getting machine UUID for %q: %w", tag, err)
	}
	machineOpenedPortRanges, err := u.portService.GetMachineOpenedPorts(ctx, machineUUID.String())
	if err != nil {
		return nil, internalerrors.Errorf("getting opened ports for machine %q: %w", tag, err)
	}
	return machineOpenedPortRanges, nil
}

// OpenedPortRangesByEndpoint returns the port ranges opened by the unit.
func (u *UniterAPI) OpenedPortRangesByEndpoint(ctx context.Context) (params.OpenPortRangesByEndpointResults, error) {
	result := params.OpenPortRangesByEndpointResults{
		Results: make([]params.OpenPortRangesByEndpointResult, 1),
	}

	authTag := u.auth.GetAuthTag()
	switch authTag.Kind() {
	case names.UnitTagKind:
	default:
		result.Results[0].Error = apiservererrors.ServerError(errors.NotSupportedf("getting opened port ranges for %q", authTag.Kind()))
		return result, nil
	}

	unitName, err := coreunit.NewName(authTag.Id())
	if err != nil {
		result.Results[0].Error = apiservererrors.ServerError(err)
		return result, nil
	}
	unitUUID, err := u.applicationService.GetUnitUUID(ctx, unitName)
	if err != nil {
		result.Results[0].Error = apiservererrors.ServerError(err)
		return result, nil
	}
	openedPortRanges, err := u.portService.GetUnitOpenedPorts(ctx, unitUUID)
	if err != nil {
		result.Results[0].Error = apiservererrors.ServerError(err)
		return result, nil
	}

	result.Results[0].UnitPortRanges = make(map[string][]params.OpenUnitPortRangesByEndpoint)
	unitTag := authTag.String()
	for endpointName, portRanges := range openedPortRanges {
		result.Results[0].UnitPortRanges[unitTag] = append(
			result.Results[0].UnitPortRanges[unitTag],
			params.OpenUnitPortRangesByEndpoint{
				Endpoint:   endpointName,
				PortRanges: transform.Slice(portRanges, params.FromNetworkPortRange),
			},
		)
	}

	// Ensure results are sorted by endpoint name to be consistent.
	sort.Slice(result.Results[0].UnitPortRanges[unitTag], func(a, b int) bool {
		return result.Results[0].UnitPortRanges[unitTag][a].Endpoint < result.Results[0].UnitPortRanges[unitTag][b].Endpoint
	})
	return result, nil
}

// AssignedMachine returns the machine tag for each given unit tag, or
// an error satisfying params.IsCodeNotAssigned when a unit has no
// assigned machine.
func (u *UniterAPI) AssignedMachine(ctx context.Context, args params.Entities) (params.StringResults, error) {
	result := params.StringResults{
		Results: make([]params.StringResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit(ctx)
	if err != nil {
		return params.StringResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		if !canAccess(tag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		unitName, err := coreunit.NewName(tag.Id())
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		machineName, err := u.applicationService.GetUnitMachineName(ctx, unitName)
		if errors.Is(err, applicationerrors.UnitMachineNotAssigned) {
			result.Results[i].Error = &params.Error{
				Code:    params.CodeNotAssigned,
				Message: err.Error(),
			}
		} else if errors.Is(err, applicationerrors.UnitNotFound) {
			result.Results[i].Error = apiservererrors.ServerError(errors.NotFoundf("unit %q", unitName))
		} else if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
		} else {
			result.Results[i].Result = names.NewMachineTag(machineName.String()).String()
		}
	}
	return result, nil
}

// PublicAddress returns the public address for each given unit, if set.
func (u *UniterAPI) PublicAddress(ctx context.Context, args params.Entities) (params.StringResults, error) {
	result := params.StringResults{
		Results: make([]params.StringResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit(ctx)
	if err != nil {
		return params.StringResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}

		if !canAccess(tag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		address, err := u.networkService.GetUnitPublicAddress(ctx, coreunit.Name(tag.Id()))
		if network.IsNoAddressError(err) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.NewNoAddressSetError(tag, "public"))
			continue
		} else if errors.Is(err, applicationerrors.UnitNotFound) {
			result.Results[i].Error = apiservererrors.ServerError(errors.NotFoundf("unit %q", tag.Id()))
			continue
		} else if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		result.Results[i].Result = address.IP().String()
	}
	return result, nil
}

// PrivateAddress returns the private address for each given unit, if set.
func (u *UniterAPI) PrivateAddress(ctx context.Context, args params.Entities) (params.StringResults, error) {
	result := params.StringResults{
		Results: make([]params.StringResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit(ctx)
	if err != nil {
		return params.StringResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}

		if !canAccess(tag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		address, err := u.networkService.GetUnitPrivateAddress(ctx, coreunit.Name(tag.Id()))
		if network.IsNoAddressError(err) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.NewNoAddressSetError(tag, "private"))
			continue
		} else if errors.Is(err, applicationerrors.UnitNotFound) {
			result.Results[i].Error = apiservererrors.ServerError(errors.NotFoundf("unit %q", tag.Id()))
			continue
		} else if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		result.Results[i].Result = address.IP().String()
	}
	return result, nil
}

// AvailabilityZone returns the availability zone for each given unit, if applicable.
func (u *UniterAPI) AvailabilityZone(ctx context.Context, args params.Entities) (params.StringResults, error) {
	var results params.StringResults

	canAccess, err := u.accessUnit(ctx)
	if err != nil {
		return results, errors.Trace(err)
	}

	// Prep the results.
	results = params.StringResults{
		Results: make([]params.StringResult, len(args.Entities)),
	}

	// Collect the zones. No zone will be collected for any entity where
	// the tag is invalid or not authorized. Instead the corresponding
	// result will be updated with the error.
	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}

		if !canAccess(tag) {
			results.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}

		unitName, err := coreunit.NewName(tag.Id())
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		machineUUID, err := u.applicationService.GetUnitMachineUUID(ctx, unitName)
		if errors.Is(err, applicationerrors.UnitNotFound) {
			results.Results[i].Error = apiservererrors.ServerError(errors.NotFoundf("unit %q", unitName))
			continue
		} else if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		az, err := u.machineService.AvailabilityZone(ctx, machineUUID)
		if errors.Is(err, machineerrors.AvailabilityZoneNotFound) {
			results.Results[i].Error = apiservererrors.ServerError(errors.NotProvisioned)
			continue
		} else if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		results.Results[i].Result = az
	}

	return results, nil
}

// WatchUnitResolveMode starts a NotifyWatcher that will send notifications
// when the reolve mode of the specified unit changes.
func (u *UniterAPI) WatchUnitResolveMode(ctx context.Context, entity params.Entity) (params.NotifyWatchResult, error) {
	canWatch, err := u.accessUnit(ctx)
	if err != nil {
		return params.NotifyWatchResult{Error: apiservererrors.ServerError(apiservererrors.ErrPerm)}, nil
	}

	tag, err := names.ParseUnitTag(entity.Tag)
	if err != nil {
		return params.NotifyWatchResult{Error: apiservererrors.ServerError(err)}, nil
	}

	if !canWatch(tag) {
		return params.NotifyWatchResult{Error: apiservererrors.ServerError(apiservererrors.ErrPerm)}, nil
	}

	unitName, err := coreunit.NewName(tag.Id())
	if err != nil {
		return params.NotifyWatchResult{Error: apiservererrors.ServerError(err)}, nil
	}

	watcher, err := u.resolveService.WatchUnitResolveMode(ctx, unitName)
	if errors.Is(err, resolveerrors.UnitNotFound) {
		return params.NotifyWatchResult{Error: apiservererrors.ServerError(errors.NotFoundf("unit %q", unitName))}, nil
	} else if err != nil {
		return params.NotifyWatchResult{Error: apiservererrors.ServerError(err)}, nil
	}

	id, _, err := internal.EnsureRegisterWatcher[struct{}](ctx, u.watcherRegistry, watcher)
	if err != nil {
		return params.NotifyWatchResult{Error: apiservererrors.ServerError(err)}, nil
	}
	return params.NotifyWatchResult{NotifyWatcherId: id}, nil
}

// Resolved returns the resolved mode for each of the given units.
func (u *UniterAPI) Resolved(ctx context.Context, args params.Entities) (params.ResolvedModeResults, error) {
	result := params.ResolvedModeResults{
		Results: make([]params.ResolvedModeResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit(ctx)
	if err != nil {
		return result, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if !canAccess(tag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		unitName, err := coreunit.NewName(tag.Id())
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		resolvedMode, err := u.resolveService.UnitResolveMode(ctx, unitName)
		if errors.Is(err, resolveerrors.UnitNotResolved) {
			result.Results[i].Mode = params.ResolvedNone
			continue
		} else if errors.Is(err, resolveerrors.UnitNotFound) {
			result.Results[i].Error = apiservererrors.ServerError(errors.NotFoundf("unit %q", unitName))
			continue
		} else if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		encodedResolveMode, err := encodeResolveMode(resolvedMode.String())
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		result.Results[i].Mode = encodedResolveMode
	}
	return result, nil
}

// ClearResolved removes any resolved setting from each given unit.
func (u *UniterAPI) ClearResolved(ctx context.Context, args params.Entities) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit(ctx)
	if err != nil {
		return params.ErrorResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		if !canAccess(tag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		unitName, err := coreunit.NewName(tag.Id())
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		err = u.resolveService.ClearResolved(ctx, unitName)
		if errors.Is(err, resolveerrors.UnitNotFound) {
			result.Results[i].Error = apiservererrors.ServerError(errors.NotFoundf("unit %q", unitName))
			continue
		} else if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
	}
	return result, nil
}

// GetPrincipal returns the result of calling PrincipalName() and
// converting it to a tag, on each given unit.
func (u *UniterAPI) GetPrincipal(ctx context.Context, args params.Entities) (params.StringBoolResults, error) {
	result := params.StringBoolResults{
		Results: make([]params.StringBoolResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit(ctx)
	if err != nil {
		return params.StringBoolResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		if !canAccess(tag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		unitName, err := coreunit.NewName(tag.Id())
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		principal, hasPrincipal, err := u.applicationService.GetUnitPrincipal(ctx, unitName)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
		} else if hasPrincipal {
			result.Results[i].Result = names.NewUnitTag(principal.String()).String()
			result.Results[i].Ok = true
		}
	}
	return result, nil
}

// Destroy advances all given Alive units' lifecycles as far as
// possible. See state/Unit.Destroy().
func (u *UniterAPI) Destroy(ctx context.Context, args params.Entities) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit(ctx)
	if err != nil {
		return params.ErrorResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		err = apiservererrors.ErrPerm
		if canAccess(tag) {
			var (
				unitName coreunit.Name
				unit     *state.Unit
				removed  bool
			)
			unitName, err = coreunit.NewName(tag.Id())
			if err != nil {
				result.Results[i].Error = apiservererrors.ServerError(err)
				continue
			}
			err = u.applicationService.DestroyUnit(ctx, unitName)
			if err != nil && !errors.Is(err, applicationerrors.UnitNotFound) {
				result.Results[i].Error = apiservererrors.ServerError(err)
				continue
			}

			// TODO(units) - remove dual write to state
			unit, err = u.getLegacyUnit(ctx, tag)
			if err == nil {
				removed, err = unit.DestroyMaybeRemove(u.store)
				if err == nil && removed {
					err = u.applicationService.DeleteUnit(ctx, unitName)
				}
			}
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

// DestroyAllSubordinates destroys all subordinates of each given unit.
func (u *UniterAPI) DestroyAllSubordinates(ctx context.Context, args params.Entities) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit(ctx)
	if err != nil {
		return params.ErrorResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		if !canAccess(tag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		unitName, err := coreunit.NewName(tag.Id())
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		err = u.destroySubordinates(ctx, unitName)
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

// HasSubordinates returns whether each given unit has any subordinates.
func (u *UniterAPI) HasSubordinates(ctx context.Context, args params.Entities) (params.BoolResults, error) {
	result := params.BoolResults{
		Results: make([]params.BoolResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit(ctx)
	if err != nil {
		return params.BoolResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		if !canAccess(tag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		unitName, err := coreunit.NewName(tag.Id())
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		subordinates, err := u.applicationService.GetUnitSubordinates(ctx, unitName)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		result.Results[i].Result = len(subordinates) > 0
	}
	return result, nil
}

// CharmModifiedVersion returns the most CharmModifiedVersion for all given
// units or applications.
func (u *UniterAPI) CharmModifiedVersion(ctx context.Context, args params.Entities) (params.IntResults, error) {
	results := params.IntResults{
		Results: make([]params.IntResult, len(args.Entities)),
	}

	accessUnitOrApplication := common.AuthAny(u.accessUnit, u.accessApplication)
	canAccess, err := accessUnitOrApplication(ctx)
	if err != nil {
		return results, err
	}
	for i, entity := range args.Entities {
		ver, err := u.charmModifiedVersion(ctx, entity.Tag, canAccess)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results.Results[i].Result = ver
	}
	return results, nil
}

func (u *UniterAPI) charmModifiedVersion(
	ctx context.Context,
	tagStr string,
	canAccess func(names.Tag) bool,
) (int, error) {
	tag, err := names.ParseTag(tagStr)
	if err != nil {
		return -1, apiservererrors.ErrPerm
	}
	if !canAccess(tag) {
		return -1, apiservererrors.ErrPerm
	}

	var id application.ID
	switch tag.(type) {
	case names.ApplicationTag:
		id, err = u.applicationService.GetApplicationIDByName(ctx, tag.Id())
		if errors.Is(err, applicationerrors.ApplicationNotFound) {
			// Return an error that also matches a generic not found error.
			return -1, internalerrors.Join(err, errors.Hide(errors.NotFound))
		} else if err != nil {
			return -1, err
		}
	case names.UnitTag:
		name, err := coreunit.NewName(tag.Id())
		if err != nil {
			return -1, err
		}
		id, err = u.applicationService.GetApplicationIDByUnitName(ctx, name)
		if errors.Is(err, applicationerrors.UnitNotFound) {
			// Return an error that also matches a generic not found error.
			return -1, internalerrors.Join(err, errors.Hide(errors.NotFound))
		} else if err != nil {
			return -1, err
		}
	default:
		return -1, errors.BadRequestf("type %s does not have a CharmModifiedVersion", tag.Kind())
	}
	charmModifiedVersion, err := u.applicationService.GetCharmModifiedVersion(ctx, id)
	if err != nil {
		return -1, err
	}
	return charmModifiedVersion, nil
}

// CharmURL returns the charm URL for all given units or applications.
//
// The "Ok" field of the result is used to indicate whether units should upgrade
// to the charm with the given URL even if they are in an error state.
func (u *UniterAPI) CharmURL(ctx context.Context, args params.Entities) (params.StringBoolResults, error) {
	result := params.StringBoolResults{
		Results: make([]params.StringBoolResult, len(args.Entities)),
	}
	accessUnitOrApplication := common.AuthAny(u.accessUnit, u.accessApplication)
	canAccess, err := accessUnitOrApplication(ctx)
	if err != nil {
		return params.StringBoolResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		if !canAccess(tag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		var (
			curl                string
			charmUpgradeOnError bool
		)
		switch t := tag.(type) {
		case names.ApplicationTag:
			curl, charmUpgradeOnError, err = u.charmURLForApplication(ctx, t)
		case names.UnitTag:
			curl, charmUpgradeOnError, err = u.charmURLForUnit(ctx, t)
		default:
			err = apiservererrors.ErrPerm
		}
		result.Results[i].Result = curl
		result.Results[i].Ok = charmUpgradeOnError
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func (u *UniterAPI) charmURLForApplication(ctx context.Context, tag names.ApplicationTag) (string, bool, error) {
	// The charmUpgradeOnError value is not used by the unit api, so
	// only set it for applications.
	charmUpgradeOnError, err := u.applicationService.ShouldAllowCharmUpgradeOnError(ctx, tag.Id())
	if err != nil {
		return "", false, internalerrors.Capture(err)
	}
	charmLocator, err := u.applicationService.GetCharmLocatorByApplicationName(ctx, tag.Id())
	if err != nil {
		return "", false, internalerrors.Capture(err)
	}
	curl, err := apiservercharms.CharmURLFromLocator(charmLocator.Name, charmLocator)
	if err != nil {
		return "", false, internalerrors.Capture(err)
	}
	return curl, charmUpgradeOnError, nil
}

func (u *UniterAPI) charmURLForUnit(ctx context.Context, tag names.UnitTag) (string, bool, error) {
	appName, err := names.UnitApplication(tag.Id())
	if err != nil {
		return "", false, internalerrors.Capture(err)
	}
	charmLocator, err := u.applicationService.GetCharmLocatorByApplicationName(ctx, appName)
	if err != nil {
		return "", false, internalerrors.Capture(err)
	}
	curl, err := apiservercharms.CharmURLFromLocator(charmLocator.Name, charmLocator)
	if err != nil {
		return "", false, internalerrors.Capture(err)
	}
	// The charmUpgradeOnError value is not used by the unit api, so always set
	// it to true for units.
	return curl, true, nil
}

// SetCharmURL sets the charm URL for each given unit. An error will
// be returned if a unit is dead, or the charm URL is not known.
func (u *UniterAPI) SetCharmURL(ctx context.Context, args params.EntitiesCharmURL) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit(ctx)
	if err != nil {
		return params.ErrorResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		err = apiservererrors.ErrPerm
		if canAccess(tag) {
			continue
			// TODO(aflynn): SetCharmURL is currently a no-op. It will be
			// addressed in the refresh epic.
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

// WorkloadVersion returns the workload version for all given units or applications.
func (u *UniterAPI) WorkloadVersion(ctx context.Context, args params.Entities) (params.StringResults, error) {
	result := params.StringResults{
		Results: make([]params.StringResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit(ctx)
	if err != nil {
		return params.StringResults{}, err
	}
	for i, entity := range args.Entities {
		resultItem := &result.Results[i]
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			resultItem.Error = apiservererrors.ServerError(err)
			continue
		}
		if !canAccess(tag) {
			resultItem.Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		unitName := coreunit.Name(tag.Id())
		version, err := u.applicationService.GetUnitWorkloadVersion(ctx, unitName)
		if errors.Is(err, applicationerrors.UnitNotFound) {
			resultItem.Error = apiservererrors.ServerError(errors.NotFoundf("unit %q", unitName))
			continue
		} else if err != nil {
			resultItem.Error = apiservererrors.ServerError(err)
			continue
		}
		resultItem.Result = version
	}
	return result, nil
}

// SetWorkloadVersion sets the workload version for each given unit. An error will
// be returned if a unit is dead.
func (u *UniterAPI) SetWorkloadVersion(ctx context.Context, args params.EntityWorkloadVersions) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit(ctx)
	if err != nil {
		return params.ErrorResults{}, err
	}
	for i, entity := range args.Entities {
		resultItem := &result.Results[i]
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			resultItem.Error = apiservererrors.ServerError(err)
			continue
		}
		if !canAccess(tag) {
			resultItem.Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		unitName := coreunit.Name(tag.Id())
		if err := u.applicationService.SetUnitWorkloadVersion(ctx, unitName, entity.WorkloadVersion); errors.Is(err, applicationerrors.UnitNotFound) {
			resultItem.Error = apiservererrors.ServerError(errors.NotFoundf("unit %q", unitName))
			continue
		} else if err != nil {
			resultItem.Error = apiservererrors.ServerError(err)
			continue
		}
	}
	return result, nil
}

// WatchActionNotifications returns a StringsWatcher for observing
// incoming action calls to a unit. See also state/watcher.go
// Unit.WatchActionNotifications(). This method is called from
// api/uniter/uniter.go WatchActionNotifications().
func (u *UniterAPI) WatchActionNotifications(ctx context.Context, args params.Entities) (params.StringsWatchResults, error) {
	// TODO (stickupkid): The actions watcher shouldn't cause the uniter to
	// start up, but here we are.
	// This is will need to fixed when actions are moved to dqlite.

	canAccess, err := u.accessUnit(ctx)
	if err != nil {
		return params.StringsWatchResults{}, err
	}

	result := params.StringsWatchResults{
		Results: make([]params.StringsWatchResult, len(args.Entities)),
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		if !canAccess(tag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}

		unitName, err := coreunit.NewName(tag.Id())
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		w, err := u.applicationService.WatchUnitActions(ctx, unitName)
		if errors.Is(err, applicationerrors.UnitNotFound) {
			result.Results[i].Error = apiservererrors.ServerError(
				errors.NotFoundf("unit %s", unitName),
			)
			continue
		} else if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		id, changes, err := internal.EnsureRegisterWatcher(ctx, u.watcherRegistry, w)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(
				internalerrors.Errorf("starting actions watcher: %w", err),
			)
			continue
		}
		result.Results[i].Changes = changes
		result.Results[i].StringsWatcherId = id
	}
	return result, nil
}

// ConfigSettings returns the complete set of application charm config
// settings available to each given unit.
func (u *UniterAPI) ConfigSettings(ctx context.Context, args params.Entities) (params.ConfigSettingsResults, error) {
	result := params.ConfigSettingsResults{
		Results: make([]params.ConfigSettingsResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit(ctx)
	if err != nil {
		return params.ConfigSettingsResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		if !canAccess(tag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}

		unitName, err := coreunit.NewName(tag.Id())
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		appID, err := u.applicationService.GetApplicationIDByUnitName(ctx, unitName)
		if errors.Is(err, applicationerrors.UnitNotFound) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		} else if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		settings, err := u.applicationService.GetApplicationConfigWithDefaults(ctx, appID)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		result.Results[i].Settings = params.ConfigSettings(settings)
	}
	return result, nil
}

// CharmArchiveSha256 returns the SHA256 digest of the charm archive
// (bundle) data for each charm url in the given parameters.
func (u *UniterAPI) CharmArchiveSha256(ctx context.Context, args params.CharmURLs) (params.StringResults, error) {
	result := params.StringResults{
		Results: make([]params.StringResult, len(args.URLs)),
	}
	for i, arg := range args.URLs {
		sha, err := u.oneCharmArchiveSha256(ctx, arg.URL)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		result.Results[i].Result = sha

	}
	return result, nil
}

func (u *UniterAPI) oneCharmArchiveSha256(ctx context.Context, curl string) (string, error) {
	locator, err := apiservercharms.CharmLocatorFromURL(curl)
	if err != nil {
		return "", errors.Trace(err)
	}

	// Only return the SHA256 if the charm is available. It is expected
	// that the caller (in this case the uniter) will retry if they get
	// a NotYetAvailable error.
	sha, err := u.applicationService.GetAvailableCharmArchiveSHA256(ctx, locator)
	if errors.Is(err, applicationerrors.CharmNotFound) {
		return "", errors.NotFoundf("charm %q", curl)
	} else if errors.Is(err, applicationerrors.CharmNotResolved) {
		return "", errors.NotYetAvailablef("charm %q not available", curl)
	} else if err != nil {
		return "", errors.Trace(err)
	}

	return sha, nil
}

// Relation returns information about all given relation/unit pairs,
// including their id, key and the local endpoint.
// v19 returns v1 RelationResults.
func (u *UniterAPIv19) Relation(ctx context.Context, args params.RelationUnits) (params.RelationResults, error) {
	result := params.RelationResults{
		Results: make([]params.RelationResult, len(args.RelationUnits)),
	}
	canAccess, err := u.accessUnit(ctx)
	if err != nil {
		return params.RelationResults{}, err
	}
	for i, rel := range args.RelationUnits {
		relParams, err := u.getOneRelation(ctx, canAccess, rel.Relation, rel.Unit)
		if err == nil {
			result.Results[i] = params.RelationResult{
				Error:            relParams.Error,
				Life:             relParams.Life,
				Suspended:        relParams.Suspended,
				Id:               relParams.Id,
				Key:              relParams.Key,
				Endpoint:         relParams.Endpoint,
				OtherApplication: relParams.OtherApplication.ApplicationName,
			}
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

// Relation returns information about all given relation/unit pairs,
// including their id, key and the local endpoint.
func (u *UniterAPI) Relation(ctx context.Context, args params.RelationUnits) (params.RelationResultsV2, error) {
	result := params.RelationResultsV2{
		Results: make([]params.RelationResultV2, len(args.RelationUnits)),
	}
	canAccess, err := u.accessUnit(ctx)
	if err != nil {
		return params.RelationResultsV2{}, err
	}
	for i, rel := range args.RelationUnits {
		relParams, err := u.getOneRelation(ctx, canAccess, rel.Relation, rel.Unit)
		if err == nil {
			result.Results[i] = relParams
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

// ActionStatus returns the status of Actions by Tags passed in.
func (u *UniterAPI) ActionStatus(ctx context.Context, args params.Entities) (params.StringResults, error) {
	canAccess, err := u.accessUnit(ctx)
	if err != nil {
		return params.StringResults{}, err
	}

	results := params.StringResults{
		Results: make([]params.StringResult, len(args.Entities)),
	}

	actionFn := common.AuthAndActionFromTagFn(canAccess, u.st.ActionByTag)
	for k, entity := range args.Entities {
		action, err := actionFn(entity.Tag)
		if err != nil {
			results.Results[k].Error = apiservererrors.ServerError(err)
			continue
		}
		results.Results[k].Result = string(action.Status())
	}

	return results, nil
}

// Actions returns the Actions by Tags passed and ensures that the Unit asking
// for them is the same Unit that has the Actions.
func (u *UniterAPI) Actions(ctx context.Context, args params.Entities) (params.ActionResults, error) {
	canAccess, err := u.accessUnit(ctx)
	if err != nil {
		return params.ActionResults{}, err
	}

	actionFn := common.AuthAndActionFromTagFn(canAccess, u.st.ActionByTag)
	return common.Actions(args, actionFn), nil
}

// BeginActions marks the actions represented by the passed in Tags as running.
func (u *UniterAPI) BeginActions(ctx context.Context, args params.Entities) (params.ErrorResults, error) {
	canAccess, err := u.accessUnit(ctx)
	if err != nil {
		return params.ErrorResults{}, err
	}

	actionFn := common.AuthAndActionFromTagFn(canAccess, u.st.ActionByTag)
	return common.BeginActions(args, actionFn), nil
}

// FinishActions saves the result of a completed Action
func (u *UniterAPI) FinishActions(ctx context.Context, args params.ActionExecutionResults) (params.ErrorResults, error) {
	canAccess, err := u.accessUnit(ctx)
	if err != nil {
		return params.ErrorResults{}, err
	}

	actionFn := common.AuthAndActionFromTagFn(canAccess, u.st.ActionByTag)
	return common.FinishActions(args, actionFn), nil
}

// LogActionsMessages records the log messages against the specified actions.
func (u *UniterAPI) LogActionsMessages(ctx context.Context, args params.ActionMessageParams) (params.ErrorResults, error) {
	canAccess, err := u.accessUnit(ctx)
	if err != nil {
		return params.ErrorResults{}, err
	}
	actionFn := common.AuthAndActionFromTagFn(canAccess, u.st.ActionByTag)

	oneActionMessage := func(actionTag string, message string) error {
		action, err := actionFn(actionTag)
		if err != nil {
			return errors.Trace(err)
		}
		return action.Log(message)
	}

	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Messages)),
	}
	for i, actionMessage := range args.Messages {
		result.Results[i].Error = apiservererrors.ServerError(
			oneActionMessage(actionMessage.Tag, actionMessage.Value))
	}
	return result, nil
}

// RelationById returns information about all given relations,
// specified by their ids, including their key and the local
// endpoint. v19 returns v1 RelationResults.
func (u *UniterAPIv19) RelationById(ctx context.Context, args params.RelationIds) (params.RelationResults, error) {
	result := params.RelationResults{
		Results: make([]params.RelationResult, len(args.RelationIds)),
	}
	for i, relId := range args.RelationIds {
		relParams, err := u.getOneRelationById(ctx, relId)
		if err == nil {
			result.Results[i] = params.RelationResult{
				Error:            relParams.Error,
				Life:             relParams.Life,
				Suspended:        relParams.Suspended,
				Id:               relParams.Id,
				Key:              relParams.Key,
				Endpoint:         relParams.Endpoint,
				OtherApplication: relParams.OtherApplication.ApplicationName,
			}
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

// RelationById returns information about all given relations,
// specified by their ids, including their key and the local
// endpoint.
func (u *UniterAPI) RelationById(ctx context.Context, args params.RelationIds) (params.RelationResultsV2, error) {
	result := params.RelationResultsV2{
		Results: make([]params.RelationResultV2, len(args.RelationIds)),
	}
	for i, relId := range args.RelationIds {
		relParams, err := u.getOneRelationById(ctx, relId)
		if err == nil {
			result.Results[i] = relParams
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

// RelationsStatus returns for each unit the corresponding relation and status information.
func (u *UniterAPI) RelationsStatus(ctx context.Context, args params.Entities) (params.RelationUnitStatusResults, error) {
	result := params.RelationUnitStatusResults{
		Results: make([]params.RelationUnitStatusResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	canRead, err := u.accessUnit(ctx)
	if err != nil {
		return params.RelationUnitStatusResults{}, err
	}

	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		err = apiservererrors.ErrPerm
		if canRead(tag) {
			result.Results[i].RelationResults, err = u.oneUnitRelationStatus(ctx, tag)
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func (u *UniterAPI) oneUnitRelationStatus(ctx context.Context, unit names.UnitTag) ([]params.RelationUnitStatus, error) {
	unitUUID, err := u.applicationService.GetUnitUUID(ctx, coreunit.Name(unit.Id()))
	if err != nil {
		return nil, internalerrors.Capture(err)
	}
	unitStatuses, err := u.relationService.GetRelationsStatusForUnit(ctx, unitUUID)
	if err != nil {
		return nil, internalerrors.Capture(err)
	}
	relationUnitStatuses := make([]params.RelationUnitStatus, len(unitStatuses))
	for i, uStatus := range unitStatuses {
		relationUnitStatuses[i] = params.RelationUnitStatus{
			RelationTag: names.NewRelationTag(uStatus.Key.String()).String(),
			InScope:     uStatus.InScope,
			Suspended:   uStatus.Suspended,
		}
	}
	return relationUnitStatuses, nil
}

// Life returns the life status of the specified applications or units.
func (u *UniterAPI) Life(ctx context.Context, args params.Entities) (params.LifeResults, error) {
	result := params.LifeResults{
		Results: make([]params.LifeResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	canRead, err := u.accessUnitOrApplication(ctx)
	if err != nil {
		return params.LifeResults{}, errors.Trace(err)
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		if !canRead(tag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		var lifeValue life.Value
		switch tag.Kind() {
		case names.ApplicationTagKind:
			lifeValue, err = u.applicationService.GetApplicationLifeByName(ctx, tag.Id())
			if errors.Is(err, applicationerrors.ApplicationNotFound) {
				err = errors.NotFoundf("application %s", tag.Id())
			}
		case names.UnitTagKind:
			var unitName coreunit.Name
			unitName, err = coreunit.NewName(tag.Id())
			if err != nil {
				result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
				continue
			}
			lifeValue, err = u.applicationService.GetUnitLife(ctx, unitName)
			if errors.Is(err, applicationerrors.UnitNotFound) {
				err = errors.NotFoundf("unit %s", unitName)
			}
		default:
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		result.Results[i].Life = lifeValue
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

// Refresh retrieves the latest values for attributes on this unit.
//
// Deprecated: Please use purpose built getters instead.
func (u *UniterAPI) Refresh(ctx context.Context, args params.Entities) (params.UnitRefreshResults, error) {
	result := params.UnitRefreshResults{
		Results: make([]params.UnitRefreshResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	canRead, err := u.accessUnit(ctx)
	if err != nil {
		return params.UnitRefreshResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {

		}
		if !canRead(tag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}

		attr, err := u.getRefresh(ctx, tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		life, err := encodeLife(attr.Life)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		resolveMode, err := encodeResolveMode(attr.ResolveMode)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		result.Results[i].Life = life
		result.Results[i].Resolved = resolveMode
		result.Results[i].ProviderID = attr.ProviderID
	}
	return result, nil
}

func encodeLife(v domainlife.Life) (life.Value, error) {
	switch v {
	case domainlife.Alive:
		return life.Alive, nil
	case domainlife.Dying:
		return life.Dying, nil
	case domainlife.Dead:
		return life.Dead, nil
	default:
		return "", errors.NotValidf("life value %q", v)
	}
}

func encodeResolveMode(v string) (params.ResolvedMode, error) {
	switch v {
	case "none":
		return params.ResolvedNone, nil
	case "no-hooks":
		return params.ResolvedNoHooks, nil
	case "retry-hooks":
		return params.ResolvedRetryHooks, nil
	default:
		return "", errors.NotValidf("resolve mode %q", v)
	}
}

func (u *UniterAPI) getRefresh(ctx context.Context, tag names.UnitTag) (domainapplication.UnitAttributes, error) {
	attributes, err := u.applicationService.GetUnitRefreshAttributes(ctx, coreunit.Name(tag.Id()))
	if errors.Is(err, applicationerrors.UnitNotFound) {
		return domainapplication.UnitAttributes{}, errors.NotFoundf("unit %s", tag)
	} else if err != nil {
		return domainapplication.UnitAttributes{}, internalerrors.Capture(err)
	}

	return attributes, nil
}

// CurrentModel returns the name and UUID for the current juju model.
func (u *UniterAPI) CurrentModel(ctx context.Context) (params.ModelResult, error) {
	result := params.ModelResult{}
	modelInfo, err := u.modelInfoService.GetModelInfo(ctx)
	if err == nil {
		result.Name = modelInfo.Name
		result.UUID = modelInfo.UUID.String()
		result.Type = modelInfo.Type.String()
	}
	return result, err
}

// ProviderType returns the provider type used by the current juju
// model.
//
// TODO(dimitern): Refactor the uniter to call this instead of calling
// ModeltConfig() just to get the provider type. Once we have machine
// addresses, this might be completely unnecessary though.
func (u *UniterAPI) ProviderType(ctx context.Context) (params.StringResult, error) {
	result := params.StringResult{}
	modelInfo, err := u.modelInfoService.GetModelInfo(ctx)
	if err == nil {
		result.Result = modelInfo.CloudType
	}
	return result, err
}

// EnterScope ensures each unit has entered its scope in the relation,
// for all of the given relation/unit pairs.
func (u *UniterAPI) EnterScope(ctx context.Context, args params.RelationUnits) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.RelationUnits)),
	}
	canAccess, err := u.accessUnit(ctx)
	if err != nil {
		return params.ErrorResults{}, err
	}

	for i, arg := range args.RelationUnits {
		tag, err := names.ParseUnitTag(arg.Unit)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		err = u.oneEnterScope(ctx, canAccess, arg.Relation, tag)
		if errors.Is(err, relationerrors.CannotEnterScopeNotAlive) {
			result.Results[i].Error = &params.Error{
				Message: err.Error(),
				Code:    params.CodeCannotEnterScope,
			}
		} else if errors.Is(err, relationerrors.CannotEnterScopeSubordinateNotAlive) {
			result.Results[i].Error = &params.Error{
				Message: err.Error(),
				Code:    params.CodeCannotEnterScopeYet,
			}
		} else if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
		}
	}
	return result, nil
}

type subordinateCreator func(ctx context.Context, subordinateAppID application.ID, principalUnitName coreunit.Name) error

// CreateSubordinate creates units on a subordinate application.
func (c subordinateCreator) CreateSubordinate(ctx context.Context, subordinateAppID application.ID, principalUnitName coreunit.Name) error {
	return c(ctx, subordinateAppID, principalUnitName)
}

func (u *UniterAPI) oneEnterScope(ctx context.Context, canAccess common.AuthFunc, relTagStr string, unitTag names.UnitTag) error {
	if !canAccess(unitTag) {
		return apiservererrors.ErrPerm
	}

	relKey, err := corerelation.ParseKeyFromTagString(relTagStr)
	if err != nil {
		return apiservererrors.ErrPerm
	}

	relUUID, err := u.relationService.GetRelationUUIDByKey(ctx, relKey)
	if internalerrors.Is(err, relationerrors.RelationNotFound) {
		return apiservererrors.ErrPerm
	} else if err != nil {
		return internalerrors.Capture(err)
	}

	unitName, err := coreunit.NewName(unitTag.Id())
	if err != nil {
		return internalerrors.Capture(err)
	}

	addr, err := u.networkService.GetUnitPublicAddress(ctx, unitName)
	if errors.Is(err, applicationerrors.UnitNotFound) {
		return errors.NotFoundf("unit %q", unitTag.Id())
	} else if err != nil {
		return internalerrors.Capture(err)
	}

	settings := map[string]string{}
	// ingress-address is the preferred settings attribute name as it more accurately
	// reflects the purpose of the attribute value. We'll deprecate private-address.
	settings["ingress-address"] = addr.String()

	err = u.relationService.EnterScope(
		ctx,
		relUUID,
		unitName,
		settings,
		subordinateCreator(u.applicationService.AddIAASSubordinateUnit),
	)
	if internalerrors.Is(err, relationerrors.PotentialRelationUnitNotValid) {
		u.logger.Debugf(ctx, "ignoring %q EnterScope for %q, not valid", unitName, relKey.String())
		return nil
	}
	return internalerrors.Capture(err)
}

// LeaveScope signals each unit has left its scope in the relation,
// for all of the given relation/unit pairs. See also
// state.RelationUnit.LeaveScope().
func (u *UniterAPI) LeaveScope(ctx context.Context, args params.RelationUnits) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.RelationUnits)),
	}
	canAccess, err := u.accessUnit(ctx)
	if err != nil {
		return params.ErrorResults{}, err
	}
	for i, arg := range args.RelationUnits {
		err := u.oneLeaveScope(ctx, canAccess, arg)
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func (u *UniterAPI) oneLeaveScope(ctx context.Context, canAccess common.AuthFunc, arg params.RelationUnit) error {
	unit, err := names.ParseUnitTag(arg.Unit)
	if err != nil {
		return err
	}
	if !canAccess(unit) {
		return apiservererrors.ErrPerm
	}
	relKey, err := corerelation.ParseKeyFromTagString(arg.Relation)
	if err != nil {
		return apiservererrors.ErrPerm
	}
	relUUID, err := u.relationService.GetRelationUUIDByKey(ctx, relKey)
	if internalerrors.Is(err, relationerrors.RelationNotFound) {
		return apiservererrors.ErrPerm
	} else if err != nil {
		return internalerrors.Capture(err)
	}
	relUnitUUID, err := u.relationService.GetRelationUnit(ctx, relUUID, coreunit.Name(unit.Id()))
	if err != nil {
		return internalerrors.Capture(err)
	}
	return u.relationService.LeaveScope(ctx, relUnitUUID)
}

// ReadSettings returns the local settings of each given set of
// relation/unit.
//
// NOTE(achilleasa): Using this call to read application data is deprecated
// and will not work for k8s charms (see LP1876097). Instead, clients should
// use ReadLocalApplicationSettings.
func (u *UniterAPI) ReadSettings(ctx context.Context, args params.RelationUnits) (params.SettingsResults, error) {
	result := params.SettingsResults{
		Results: make([]params.SettingsResult, len(args.RelationUnits)),
	}
	canAccessUnit, err := u.accessUnit(ctx)
	if err != nil {
		return params.SettingsResults{}, errors.Trace(err)
	}
	for i, arg := range args.RelationUnits {
		settings, err := u.readOneUnitSettings(ctx, canAccessUnit, arg)
		result.Results[i].Error = apiservererrors.ServerError(err)
		result.Results[i].Settings = settings
	}
	return result, nil
}

func (u *UniterAPI) readOneUnitSettings(
	ctx context.Context,
	canAccessUnit common.AuthFunc,
	arg params.RelationUnit,
) (params.Settings, error) {
	unitTag, err := names.ParseTag(arg.Unit)
	if err != nil {
		return nil, apiservererrors.ErrPerm
	}
	relKey, err := corerelation.ParseKeyFromTagString(arg.Relation)
	if err != nil {
		return nil, apiservererrors.ErrPerm
	}
	relUUID, err := u.relationService.GetRelationUUIDByKey(ctx, relKey)
	if errors.Is(err, relationerrors.RelationNotFound) {
		return nil, apiservererrors.ErrPerm
	} else if err != nil {
		return nil, internalerrors.Capture(err)
	}

	var settings map[string]string

	switch tag := unitTag.(type) {
	case names.UnitTag:
		settings, err = u.readLocalUnitSettings(ctx, canAccessUnit, relUUID, tag)
		if err != nil {
			return nil, internalerrors.Capture(err)
		}
	case names.ApplicationTag:
		// Emulate a ReadLocalApplicationSettings call where
		// the currently authenticated wordpressUnitTag is implicitly
		// assumed to be the requesting unit.
		authTag := u.auth.GetAuthTag()
		if authTag.Kind() != names.UnitTagKind {
			// See LP1876097
			return nil, apiservererrors.ErrPerm
		}
		unitName, err := coreunit.NewName(authTag.Id())
		if err != nil {
			return nil, internalerrors.Capture(err)
		}
		settings, err = u.readLocalApplicationSettings(ctx, relUUID, tag, unitName)
		if err != nil {
			return nil, internalerrors.Capture(err)
		}
	default:
		return nil, apiservererrors.ErrPerm
	}
	return settings, nil
}

func (u *UniterAPI) readLocalUnitSettings(
	ctx context.Context,
	canAccess common.AuthFunc,
	relUUID corerelation.UUID,
	unitTag names.UnitTag,
) (map[string]string, error) {
	if !canAccess(unitTag) {
		return nil, apiservererrors.ErrPerm
	}

	unitName, err := coreunit.NewName(unitTag.Id())
	if err != nil {
		return nil, internalerrors.Capture(err)
	}
	relUnitUUID, err := u.relationService.GetRelationUnit(ctx, relUUID, unitName)
	if err != nil {
		return nil, internalerrors.Capture(err)
	}

	return u.relationService.GetRelationUnitSettings(ctx, relUnitUUID)
}

// ReadLocalApplicationSettings returns the local application settings for a
// particular relation when invoked by the leader unit.
func (u *UniterAPI) ReadLocalApplicationSettings(ctx context.Context, arg params.RelationUnit) (params.SettingsResult, error) {
	var res params.SettingsResult

	unitTag, err := names.ParseUnitTag(arg.Unit)
	if err != nil {
		return res, errors.NotValidf("unit tag %q", arg.Unit)
	}
	relKey, err := corerelation.ParseKeyFromTagString(arg.Relation)
	if err != nil {
		return res, errors.NotValidf("relation tag %q", arg.Relation)
	}

	inferredAppName, err := names.UnitApplication(unitTag.Id())
	if err != nil {
		return res, errors.NotValidf("inferred application name from %q", arg.Unit)
	}
	inferredAppTag := names.NewApplicationTag(inferredAppName)

	unitName, err := coreunit.NewName(unitTag.Id())
	if err != nil {
		return res, apiservererrors.ServerError(err)
	}

	// Check whether the agent has authenticated as a unit or as an
	// application (e.g. an operator in a k8s scenario).
	authTag := u.auth.GetAuthTag()
	switch authTag.Kind() {
	case names.UnitTagKind:
		// In this case, the authentication tag must match the unit tag
		// provided by the caller.
		if authTag.String() != unitTag.String() {
			return res, errors.Trace(apiservererrors.ErrPerm)
		}
	case names.ApplicationTagKind:
		// In this case (k8s operator), we have no alternative than to
		// implicitly trust the unit tag argument passed in by the
		// operator (for more details, see LP1876097). As an early
		// sanity check, ensure that the inferred application from the
		// unit name matches the one currently logged on.
		if authTag.String() != inferredAppTag.String() {
			return res, errors.Trace(apiservererrors.ErrPerm)
		}
	default:
		return res, errors.NotSupportedf("reading local application settings after authenticating as %q", authTag.Kind())
	}

	relUUID, err := u.relationService.GetRelationUUIDByKey(ctx, relKey)
	if errors.Is(err, relationerrors.RelationNotFound) {
		return res, apiservererrors.ErrPerm
	} else if err != nil {
		return res, internalerrors.Capture(err)
	}

	settings, err := u.readLocalApplicationSettings(ctx, relUUID, inferredAppTag, unitName)
	if err != nil {
		return res, errors.Trace(err)
	}
	res.Settings = settings
	return res, errors.Trace(err)
}

// readLocalApplicationSettings attempts to access the local application data
// bag for the specified relation on based on an ApplicationTag on behalf of
// a UnitTag.
func (u *UniterAPI) readLocalApplicationSettings(
	ctx context.Context,
	relUUID corerelation.UUID,
	appTag names.ApplicationTag,
	unitName coreunit.Name,
) (map[string]string, error) {
	canAccessApp, err := u.accessApplication(ctx)
	if err != nil {
		return nil, internalerrors.Capture(err)
	}

	if !canAccessApp(appTag) {
		return nil, apiservererrors.ErrPerm
	}

	appID, err := u.applicationService.GetApplicationIDByName(ctx, appTag.Id())
	if errors.Is(err, errors.NotFound) {
		return nil, apiservererrors.ErrPerm
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	settings, err := u.relationService.GetRelationApplicationSettingsWithLeader(ctx, unitName, relUUID, appID)
	if errors.Is(err, corelease.ErrNotHeld) || errors.Is(err, relationerrors.RelationNotFound) || errors.Is(err, relationerrors.ApplicationNotFoundForRelation) {
		return nil, apiservererrors.ErrPerm
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	return settings, nil
}

// ReadRemoteSettings returns the remote settings of each given set of
// relation/local unit/remote unit.
func (u *UniterAPI) ReadRemoteSettings(ctx context.Context, args params.RelationUnitPairs) (params.SettingsResults, error) {
	result := params.SettingsResults{
		Results: make([]params.SettingsResult, len(args.RelationUnitPairs)),
	}
	canAccess, err := u.accessUnit(ctx)
	if err != nil {
		return params.SettingsResults{}, err
	}
	for i, arg := range args.RelationUnitPairs {
		settings, err := u.readOneRemoteSettings(ctx, canAccess, arg)
		result.Results[i].Error = apiservererrors.ServerError(err)
		result.Results[i].Settings = settings
	}

	return result, nil
}

func (u *UniterAPI) readOneRemoteSettings(ctx context.Context, canAccess common.AuthFunc, arg params.RelationUnitPair) (params.Settings, error) {
	unitTag, err := names.ParseUnitTag(arg.LocalUnit)
	if err != nil {
		return nil, apiservererrors.ErrPerm
	}

	if !canAccess(unitTag) {
		return nil, apiservererrors.ErrPerm
	}

	remoteTag, err := names.ParseTag(arg.RemoteUnit)
	if err != nil {
		return nil, apiservererrors.ErrPerm
	}
	relationKey, err := corerelation.ParseKeyFromTagString(arg.Relation)
	if err != nil {
		return nil, apiservererrors.ErrPerm
	}

	relUUID, err := u.relationService.GetRelationUUIDByKey(ctx, relationKey)
	if err != nil {
		return nil, err
	}

	var settings map[string]string

	switch remoteTag := remoteTag.(type) {
	case names.UnitTag:
		remoteUnitName, err := coreunit.NewName(remoteTag.Id())
		if err != nil {
			return nil, internalerrors.Capture(err)
		}
		relUnitUUID, err := u.relationService.GetRelationUnit(ctx, relUUID, remoteUnitName)
		if err != nil {
			return nil, internalerrors.Capture(err)
		}
		settings, err = u.relationService.GetRelationUnitSettings(ctx, relUnitUUID)
		if err != nil {
			return nil, internalerrors.Capture(err)
		}
	case names.ApplicationTag:
		remoteAppID, err := u.applicationService.GetApplicationIDByName(ctx, remoteTag.Id())
		if err != nil {
			return nil, internalerrors.Capture(err)
		}
		settings, err = u.relationService.GetRelationApplicationSettings(ctx, relUUID, remoteAppID)
		if err != nil {
			return nil, internalerrors.Capture(err)
		}
	default:
		return nil, apiservererrors.ErrPerm
	}
	return settings, nil
}

func (u *UniterAPI) updateUnitAndApplicationSettings(ctx context.Context, arg params.RelationUnitSettings, canAccess common.AuthFunc) error {
	unitTag, err := names.ParseUnitTag(arg.Unit)
	if err != nil {
		return apiservererrors.ErrPerm
	}
	if !canAccess(unitTag) {
		return apiservererrors.ErrPerm
	}
	relKey, err := corerelation.ParseKeyFromTagString(arg.Relation)
	if err != nil {
		return apiservererrors.ErrPerm
	}
	relUUID, err := u.relationService.GetRelationUUIDByKey(ctx, relKey)
	if err != nil {
		return internalerrors.Capture(err)
	}
	unitName := coreunit.Name(unitTag.Id())

	relUnitUUID, err := u.relationService.GetRelationUnit(ctx, relUUID, unitName)
	if err != nil {
		return internalerrors.Capture(err)
	}

	err = u.relationService.SetRelationApplicationAndUnitSettings(ctx, unitName, relUnitUUID, arg.ApplicationSettings, arg.Settings)
	if errors.Is(err, corelease.ErrNotHeld) {
		return apiservererrors.ErrPerm
	} else if err != nil {
		return internalerrors.Capture(err)
	}

	return nil
}

// WatchRelationUnits returns a RelationUnitsWatcher for observing
// changes to every unit in the supplied relation that is visible to
// the supplied unit.
func (u *UniterAPI) WatchRelationUnits(ctx context.Context, args params.RelationUnits) (params.RelationUnitsWatchResults, error) {
	result := params.RelationUnitsWatchResults{
		Results: make([]params.RelationUnitsWatchResult, len(args.RelationUnits)),
	}
	canAccess, err := u.accessUnit(ctx)
	if err != nil {
		return params.RelationUnitsWatchResults{}, err
	}
	for i, arg := range args.RelationUnits {
		result.Results[i], err = u.watchOneRelationUnit(ctx, canAccess, arg)
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func (u *UniterAPI) watchOneRelationUnit(
	ctx context.Context,
	canAccess common.AuthFunc,
	arg params.RelationUnit,
) (params.RelationUnitsWatchResult, error) {
	unit, err := names.ParseUnitTag(arg.Unit)
	if err != nil {
		return params.RelationUnitsWatchResult{}, apiservererrors.ErrPerm
	}
	if !canAccess(unit) {
		return params.RelationUnitsWatchResult{}, apiservererrors.ErrPerm
	}
	relKey, err := corerelation.ParseKeyFromTagString(arg.Relation)
	if err != nil {
		return params.RelationUnitsWatchResult{}, apiservererrors.ErrPerm
	}
	relUUID, err := u.relationService.GetRelationUUIDByKey(ctx, relKey)
	if internalerrors.Is(err, relationerrors.RelationNotFound) {
		return params.RelationUnitsWatchResult{}, apiservererrors.ErrPerm
	} else if err != nil {
		return params.RelationUnitsWatchResult{}, internalerrors.Capture(err)
	}

	watch, err := newRelationUnitsWatcher(unit, relUUID, u.relationService)
	if err != nil {
		return params.RelationUnitsWatchResult{},
			internalerrors.Capture(internalerrors.Errorf("starting related units watcher: %w", err))
	}

	id, changes, err := internal.EnsureRegisterWatcher[params.RelationUnitsChange](
		ctx,
		u.watcherRegistry,
		watch,
	)
	if err != nil {
		return params.RelationUnitsWatchResult{},
			internalerrors.Capture(internalerrors.Errorf("registering related units watcher : %w", err))
	}

	return params.RelationUnitsWatchResult{
		RelationUnitsWatcherId: id,
		Changes:                changes,
	}, nil
}

// SetRelationStatus updates the status of the specified relations.
func (u *UniterAPI) SetRelationStatus(ctx context.Context, args params.RelationStatusArgs) (params.ErrorResults, error) {
	var statusResults params.ErrorResults
	results := make([]params.ErrorResult, len(args.Args))
	for i, arg := range args.Args {
		unitTag, err := names.ParseUnitTag(arg.UnitTag)
		if err != nil {
			results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		unitName, err := coreunit.NewName(unitTag.Id())
		if err != nil {
			results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		err = u.oneSetRelationStatus(ctx, unitName, arg.RelationId,
			arg.Status, arg.Message)
		results[i].Error = apiservererrors.ServerError(err)
	}
	statusResults.Results = results
	return statusResults, nil
}

func (u *UniterAPI) oneSetRelationStatus(
	ctx context.Context,
	unitName coreunit.Name,
	relID int,
	relStatus params.RelationStatusValue,
	message string,
) error {
	// Verify the relation exist before continuing.
	relationUUID, err := u.relationService.GetRelationUUIDByID(ctx, relID)
	if errors.Is(err, relationerrors.RelationNotFound) {
		return apiservererrors.ErrPerm
	} else if err != nil {
		return internalerrors.Capture(err)
	}

	err = u.statusService.SetRelationStatus(ctx, unitName, relationUUID, status.StatusInfo{
		Status:  status.Status(relStatus),
		Message: message,
		Since:   ptr(u.clock.Now()),
	})
	if errors.Is(err, errors.NotFound) {
		return apiservererrors.ErrPerm
	} else if err != nil {
		return internalerrors.Capture(err)
	}
	return nil
}

func (u *UniterAPI) getLegacyUnit(ctx context.Context, tag names.UnitTag) (*state.Unit, error) {
	return u.st.Unit(tag.Id())
}

func (u *UniterAPI) getOneRelationById(ctx context.Context, relID int) (params.RelationResultV2, error) {
	nothing := params.RelationResultV2{}
	relUUID, err := u.relationService.GetRelationUUIDByID(ctx, relID)
	if errors.Is(err, relationerrors.RelationNotFound) {
		return nothing, apiservererrors.ErrPerm
	} else if err != nil {
		return nothing, err
	}
	rel, err := u.relationService.GetRelationDetails(ctx, relUUID)
	if errors.Is(err, relationerrors.RelationNotFound) {
		return nothing, apiservererrors.ErrPerm
	} else if err != nil {
		return nothing, err
	}
	var applicationName string
	tag := u.auth.GetAuthTag()
	switch tag.(type) {
	case names.UnitTag:
		applicationName, err = names.UnitApplication(tag.Id())
		if err != nil {
			return nothing, err
		}
	case names.ApplicationTag:
		applicationName = tag.Id()
	default:
		panic("authenticated entity is not a unit or application")
	}
	// Use the currently authenticated unit to get the endpoint.
	result, err := u.prepareRelationResult(rel, applicationName)
	if err != nil {
		// An error from prepareRelationResult means the authenticated
		// unit's application is not part of the requested
		// relation. That's why it's appropriate to return ErrPerm
		// here.
		return nothing, apiservererrors.ErrPerm
	}
	return result, nil
}

func (u *UniterAPI) prepareRelationResult(
	rel relation.RelationDetails,
	applicationName string,
) (params.RelationResultV2, error) {
	var (
		otherAppName string
		unitEp       relation.Endpoint
	)
	for _, v := range rel.Endpoints {
		if v.ApplicationName == applicationName {
			unitEp = v
		} else {
			otherAppName = v.ApplicationName
		}
	}
	// Only an application in the relation can request this data.
	if unitEp.ApplicationName != applicationName {
		return params.RelationResultV2{},
			internalerrors.Errorf("application %q is not part of the relation", applicationName)
	}
	otherApplication := params.RelatedApplicationDetails{
		ApplicationName: otherAppName,
		ModelUUID:       u.modelUUID.String(),
	}
	return params.RelationResultV2{
		Id:   rel.ID,
		Key:  rel.Key.String(),
		Life: rel.Life,
		Endpoint: params.Endpoint{
			ApplicationName: unitEp.ApplicationName,
			Relation:        params.NewCharmRelation(unitEp.Relation),
		},
		OtherApplication: otherApplication,
	}, nil
}

func (u *UniterAPI) getOneRelation(
	ctx context.Context,
	canAccess common.AuthFunc,
	relTagStr, unitTagStr string,
) (params.RelationResultV2, error) {
	nothing := params.RelationResultV2{}
	unitTag, err := names.ParseUnitTag(unitTagStr)
	if err != nil {
		return nothing, apiservererrors.ErrPerm
	}
	if !canAccess(unitTag) {
		return nothing, apiservererrors.ErrPerm
	}
	relationKey, err := corerelation.ParseKeyFromTagString(relTagStr)
	if err != nil {
		return nothing, apiservererrors.ErrPerm
	}
	relUUID, err := u.relationService.GetRelationUUIDByKey(ctx, relationKey)
	if errors.Is(err, relationerrors.RelationNotFound) {
		return nothing, apiservererrors.ErrPerm
	} else if err != nil {
		return nothing, err
	}
	rel, err := u.relationService.GetRelationDetails(ctx, relUUID)
	if errors.Is(err, relationerrors.RelationNotFound) {
		return nothing, apiservererrors.ErrPerm
	} else if err != nil {
		return nothing, err
	}
	appName, err := names.UnitApplication(unitTag.Id())
	if err != nil {
		return nothing, apiservererrors.ErrBadId
	}
	return u.prepareRelationResult(rel, appName)
}

func (u *UniterAPI) destroySubordinates(ctx context.Context, principal coreunit.Name) error {
	subordinates, err := u.applicationService.GetUnitSubordinates(ctx, principal)
	if err != nil {
		return internalerrors.Capture(err)
	}
	for _, subName := range subordinates {
		err = u.applicationService.DestroyUnit(ctx, subName)
		if err != nil && !errors.Is(err, applicationerrors.UnitNotFound) {
			return internalerrors.Capture(err)
		}

		// TODO(units) - remove dual write to state
		unit, err := u.getLegacyUnit(ctx, names.NewUnitTag(subName.String()))
		if err != nil {
			return err
		}
		removed, err := unit.DestroyMaybeRemove(u.store)
		if err != nil {
			return err
		}
		if removed {
			if err := u.applicationService.DeleteUnit(ctx, subName); err != nil {
				return err
			}
		}
	}
	return nil
}

// NetworkInfo returns network interfaces/addresses for specified bindings.
func (u *UniterAPI) NetworkInfo(ctx context.Context, args params.NetworkInfoParams) (params.NetworkInfoResults, error) {
	canAccess, err := u.accessUnit(ctx)
	if err != nil {
		return params.NetworkInfoResults{}, err
	}

	unitTag, err := names.ParseUnitTag(args.Unit)
	if err != nil {
		return params.NetworkInfoResults{}, err
	}

	if !canAccess(unitTag) {
		return params.NetworkInfoResults{}, apiservererrors.ErrPerm
	}

	addr, err := u.networkService.GetUnitPublicAddress(ctx, coreunit.Name(unitTag.Id()))
	if errors.Is(err, applicationerrors.UnitNotFound) {
		return params.NetworkInfoResults{}, errors.NotFoundf("unit %q", unitTag.Id())
	} else if err != nil {
		return params.NetworkInfoResults{}, internalerrors.Capture(err)
	}

	results := params.NetworkInfoResults{
		Results: make(map[string]params.NetworkInfoResult),
	}
	for _, binding := range args.Endpoints {
		results.Results[binding] = params.NetworkInfoResult{
			IngressAddresses: []string{addr.String()},
		}
	}
	return results, nil
}

// WatchUnitRelations returns a StringsWatcher, for each given
// unit, that notifies of changes to the lifecycles of relations
// relevant to that unit. For principal units, this will be all of the
// relations for the application. For subordinate units, only
// relations with the principal unit's application will be monitored.
func (u *UniterAPI) WatchUnitRelations(ctx context.Context, args params.Entities) (params.StringsWatchResults, error) {
	result := params.StringsWatchResults{
		Results: make([]params.StringsWatchResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit(ctx)
	if err != nil {
		return params.StringsWatchResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		err = apiservererrors.ErrPerm
		if canAccess(tag) {
			result.Results[i], err = u.watchOneUnitRelations(ctx, tag)
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func (u *UniterAPI) watchOneUnitRelations(ctx context.Context, tag names.UnitTag) (params.StringsWatchResult, error) {
	nothing := params.StringsWatchResult{}

	unitName, err := coreunit.NewName(tag.Id())
	if err != nil {
		return nothing, err
	}

	unitUUID, err := u.applicationService.GetUnitUUID(ctx, unitName)
	if err != nil {
		return nothing, err
	}

	watch, err := u.relationService.WatchLifeSuspendedStatus(ctx, unitUUID)
	if err != nil {
		updatedError := internalerrors.Errorf("WatchUnitRelations for %q: %w",
			unitName, err)
		return nothing, internalerrors.Capture(updatedError)
	}

	watcherId, initial, err := internal.EnsureRegisterWatcher[[]string](ctx, u.watcherRegistry, watch)
	if err != nil {
		return nothing, nil
	}
	return params.StringsWatchResult{StringsWatcherId: watcherId, Changes: initial}, nil
}

func makeAppAuthChecker(authTag names.Tag) common.AuthFunc {
	return func(tag names.Tag) bool {
		if tag, ok := tag.(names.ApplicationTag); ok {
			switch authTag.(type) {
			case names.UnitTag:
				appName, err := names.UnitApplication(authTag.Id())
				return err == nil && appName == tag.Id()
			case names.ApplicationTag:
				return tag == authTag
			}
		}
		return false
	}
}

// CloudSpec returns the cloud spec used by the model in which the
// authenticated unit resides.
// A check is made beforehand to ensure that the request is made by a unit
// that has been granted the appropriate trust.
func (u *UniterAPI) CloudSpec(ctx context.Context) (params.CloudSpecResult, error) {
	// Check access - any error will be permission denied.
	canAccess, err := u.accessCloudSpec(ctx)
	if err != nil {
		return params.CloudSpecResult{Error: apiservererrors.ServerError(apiservererrors.ErrPerm)}, nil
	}
	if !canAccess() {
		return params.CloudSpecResult{Error: apiservererrors.ServerError(apiservererrors.ErrPerm)}, nil
	}

	spec, err := u.modelProviderService.GetCloudSpec(ctx)
	if err != nil {
		return params.CloudSpecResult{}, errors.Trace(err)
	}
	return params.CloudSpecResult{
		Result: common.CloudSpecToParams(spec),
	}, nil
}

// GoalStates returns information of charm units and relations.
//
// TODO(jack-w-shaw): This endpoint is very complex. It's implementation should
// be pushed into into the domain layer.
func (u *UniterAPI) GoalStates(ctx context.Context, args params.Entities) (params.GoalStateResults, error) {
	result := params.GoalStateResults{
		Results: make([]params.GoalStateResult, len(args.Entities)),
	}

	canAccess, err := u.accessUnit(ctx)
	if err != nil {
		return params.GoalStateResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		if !canAccess(tag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		unitName, err := coreunit.NewName(tag.Id())
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		result.Results[i].Result, err = u.oneGoalState(ctx, unitName)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
		}
	}
	return result, nil
}

// oneGoalState creates the goal state for a given unit.
func (u *UniterAPI) oneGoalState(ctx context.Context, unitName coreunit.Name) (*params.GoalState, error) {
	appName := unitName.Application()

	appID, err := u.applicationService.GetApplicationIDByUnitName(ctx, unitName)
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		return nil, errors.NotFoundf("application %q", appName)
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	gs := params.GoalState{}
	gs.Units, err = u.goalStateUnits(ctx, appName, appID, unitName)
	if err != nil {
		return nil, err
	}

	allRelations, err := u.relationService.GetGoalStateRelationDataForApplication(ctx, appID)
	if err != nil {
		return nil, err
	}

	gs.Relations, err = u.goalStateRelations(ctx, appName, unitName, allRelations)
	if err != nil {
		return nil, err
	}

	return &gs, nil
}

// goalStateRelations creates the structure with all the relations between endpoints in an application.
func (u *UniterAPI) goalStateRelations(
	ctx context.Context,
	baseAppName string,
	principalName coreunit.Name,
	allRelations []relation.GoalStateRelationData,
) (map[string]params.UnitsGoalState, error) {
	result := map[string]params.UnitsGoalState{}

	for _, rel := range allRelations {
		endPoints := rel.EndpointIdentifiers
		if len(endPoints) == 1 {
			// Ignore peer relations here.
			continue
		}

		// First determine the local endpoint name to use later
		// as the key in the result map.
		var resultEndpointName string
		for _, e := range endPoints {
			if e.ApplicationName == baseAppName {
				resultEndpointName = e.EndpointName
			}
		}
		if resultEndpointName == "" {
			continue
		}

		// Now gather the goal state.
		for _, e := range endPoints {
			var appName string
			appID, err := u.applicationService.GetApplicationIDByName(ctx, e.ApplicationName)
			if err == nil {
				appName = e.ApplicationName
			} else if errors.Is(err, applicationerrors.ApplicationNotFound) {
				u.logger.Debugf(ctx, "application %q must be a remote application.", e.ApplicationName)
				// TODO(jack-w-shaw): Once CMRs have been implemented in DQLite,
				// set the appName to the remote application URL.
				continue
			} else {
				return nil, err
			}

			// We don't show units for the same application as we are currently processing.
			if appName == baseAppName {
				continue
			}

			goalState := params.GoalStateStatus{}
			goalState.Status = rel.Status.String()
			goalState.Since = rel.Since
			relationGoalState := result[e.EndpointName]
			if relationGoalState == nil {
				relationGoalState = params.UnitsGoalState{}
			}
			relationGoalState[appName] = goalState

			units, err := u.goalStateUnits(ctx, appName, appID, principalName)
			if err != nil {
				return nil, err
			}
			for unitName, unitGS := range units {
				relationGoalState[unitName] = unitGS
			}

			// Merge in the goal state for the current remote endpoint
			// with any other goal state already collected for the local endpoint.
			unitsGoalState := result[resultEndpointName]
			if unitsGoalState == nil {
				unitsGoalState = params.UnitsGoalState{}
			}
			for k, v := range relationGoalState {
				unitsGoalState[k] = v
			}
			result[resultEndpointName] = unitsGoalState
		}
	}
	return result, nil
}

// goalStateUnits loops through all application units related to principalName,
// and stores the goal state status in UnitsGoalState.
func (u *UniterAPI) goalStateUnits(ctx context.Context, appName string, appID application.ID, principalName coreunit.Name) (params.UnitsGoalState, error) {

	allUnitNames, err := u.applicationService.GetUnitNamesForApplication(ctx, appName)
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		return nil, errors.NotFoundf("application %q", appName)
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	unitWorkloadStatuses, err := u.statusService.GetUnitWorkloadStatusesForApplication(ctx, appID)
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		return nil, errors.NotFoundf("application %q", appName)
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	unitsGoalState := params.UnitsGoalState{}
	for _, unitName := range allUnitNames {
		pn, hasPrincipal, err := u.applicationService.GetUnitPrincipal(ctx, unitName)
		if errors.Is(err, applicationerrors.UnitNotFound) {
			return nil, errors.NotFoundf("unit %q", unitName)
		} else if err != nil {
			return nil, internalerrors.Errorf("getting principal for unit %q: %w", unitName, err)
		}
		if hasPrincipal && pn != principalName {
			continue
		}

		unitLife, err := u.applicationService.GetUnitLife(ctx, unitName)
		if errors.Is(err, applicationerrors.UnitNotFound) {
			return nil, errors.NotFoundf("unit %q", unitName)
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		if unitLife == life.Dead {
			// only show Alive and Dying units
			u.logger.Debugf(ctx, "unit %q is dead, ignore it.", unitName)
			continue
		}
		unitGoalState := params.GoalStateStatus{}
		statusInfo, ok := unitWorkloadStatuses[unitName]
		if !ok {
			return nil, errors.Errorf("status for unit %q not found", unitName)
		}
		unitGoalState.Status = statusInfo.Status.String()
		if unitLife == life.Dying {
			unitGoalState.Status = string(unitLife)
		}
		unitGoalState.Since = statusInfo.Since
		unitsGoalState[unitName.String()] = unitGoalState
	}

	return unitsGoalState, nil
}

// WatchConfigSettingsHash returns a StringsWatcher that yields a hash
// of the config values every time the config changes. The uniter can
// save this hash and use it to decide whether the config-changed hook
// needs to be run (or whether this was just an agent restart with no
// substantive config change).
func (u *UniterAPI) WatchConfigSettingsHash(ctx context.Context, args params.Entities) (params.StringsWatchResults, error) {
	getWatcher := func(ctx context.Context, unitName coreunit.Name) (watcher.StringsWatcher, error) {
		return u.applicationService.WatchApplicationConfigHash(ctx, unitName.Application())
	}
	result, err := u.watchHashes(ctx, args, getWatcher)
	if err != nil {
		return params.StringsWatchResults{}, errors.Trace(err)
	}
	return result, nil
}

// WatchTrustConfigSettingsHash returns a StringsWatcher that yields a
// hash of the application config values whenever they change. The
// uniter can use the hash to determine whether the actual values have
// changed since it last saw the config.
func (u *UniterAPI) WatchTrustConfigSettingsHash(ctx context.Context, args params.Entities) (params.StringsWatchResults, error) {
	getWatcher := func(ctx context.Context, unitName coreunit.Name) (watcher.StringsWatcher, error) {
		return u.applicationService.WatchApplicationConfigHash(ctx, unitName.Application())
	}
	result, err := u.watchHashes(ctx, args, getWatcher)
	if err != nil {
		return params.StringsWatchResults{}, errors.Trace(err)
	}
	return result, nil
}

// WatchUnitAddressesHash returns a StringsWatcher that yields the
// hashes of the addresses for the unit whenever the addresses
// change. The uniter can use the hash to determine whether the actual
// address values have changed since it last saw the config.
func (u *UniterAPI) WatchUnitAddressesHash(ctx context.Context, args params.Entities) (params.StringsWatchResults, error) {
	result, err := u.watchHashes(ctx, args, u.applicationService.WatchUnitAddressesHash)
	if err != nil {
		return params.StringsWatchResults{}, errors.Trace(err)
	}
	return result, nil
}

func (u *UniterAPI) watchHashes(ctx context.Context, args params.Entities, getWatcher func(ctx context.Context, unitName coreunit.Name) (watcher.StringsWatcher, error)) (params.StringsWatchResults, error) {
	result := params.StringsWatchResults{
		Results: make([]params.StringsWatchResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit(ctx)
	if err != nil {
		return params.StringsWatchResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		err = apiservererrors.ErrPerm
		watcherId := ""
		var changes []string
		if canAccess(tag) {
			watcherId, changes, err = u.watchOneUnitHashes(ctx, tag, getWatcher)
		}
		result.Results[i].StringsWatcherId = watcherId
		result.Results[i].Changes = changes
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func (u *UniterAPI) watchOneUnitHashes(ctx context.Context, tag names.UnitTag, getWatcher func(ctx context.Context, unitName coreunit.Name) (watcher.StringsWatcher, error)) (string, []string, error) {
	unitName, err := coreunit.NewName(tag.Id())
	if err != nil {
		return "", nil, internalerrors.Capture(err)
	}
	w, err := getWatcher(ctx, unitName)
	if errors.Is(err, applicationerrors.UnitNotFound) {
		return "", nil, errors.NotFoundf("unit %q", unitName)
	}
	if err != nil {
		return "", nil, internalerrors.Capture(err)
	}

	id, changes, err := internal.EnsureRegisterWatcher[[]string](ctx, u.watcherRegistry, w)
	if err != nil {
		return "", nil, internalerrors.Errorf("starting hash watcher: %w", err)
	}

	return id, changes, nil
}

// CloudAPIVersion returns the cloud API version, if available.
func (u *UniterAPI) CloudAPIVersion(ctx context.Context) (params.StringResult, error) {
	result := params.StringResult{}

	apiVersion, err := u.modelInfoService.CloudAPIVersion(ctx)
	if err != nil {
		return result, apiservererrors.ServerError(err)
	}
	result.Result = apiVersion
	return result, err
}

// UpdateNetworkInfo refreshes the network settings for a unit's bound
// endpoints.
func (u *UniterAPI) UpdateNetworkInfo(ctx context.Context, args params.Entities) (params.ErrorResults, error) {
	// TODO hmlanigan 2025-04-09
	// Implement with the link layer devices domain.
	return params.ErrorResults{}, nil
}

// CommitHookChanges batches together all required API calls for applying
// a set of changes after a hook successfully completes and executes them in a
// single transaction.
func (u *UniterAPI) CommitHookChanges(ctx context.Context, args params.CommitHookChangesArgs) (params.ErrorResults, error) {
	canAccessUnit, err := u.accessUnit(ctx)
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}

	canAccessApp := makeAppAuthChecker(u.auth.GetAuthTag())

	res := make([]params.ErrorResult, len(args.Args))
	for i, arg := range args.Args {
		unitTag, err := names.ParseUnitTag(arg.Tag)
		if err != nil {
			res[i].Error = apiservererrors.ServerError(err)
			continue
		}

		if !canAccessUnit(unitTag) {
			res[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}

		if err := u.commitHookChangesForOneUnit(ctx, unitTag, arg, canAccessUnit, canAccessApp); err != nil {
			// Log quota-related errors to aid operators
			if errors.Is(err, errors.QuotaLimitExceeded) {
				u.logger.Errorf(ctx, "%s: %v", unitTag, err)
			}
			res[i].Error = apiservererrors.ServerError(err)
		}
	}

	return params.ErrorResults{Results: res}, nil
}

func (u *UniterAPI) commitHookChangesForOneUnit(
	ctx context.Context,
	unitTag names.UnitTag,
	changes params.CommitHookChangesArg,
	canAccessUnit, canAccessApp common.AuthFunc,
) error {
	var modelOps []state.ModelOperation

	if changes.UpdateNetworkInfo {
		// TODO hmlanigan 2025-04-09
		// Implement with the link layer devices domain.
	}

	for _, rus := range changes.RelationUnitSettings {
		// Ensure the unit in the unit settings matches the root unit name
		if rus.Unit != changes.Tag {
			return apiservererrors.ErrPerm
		}
		err := u.updateUnitAndApplicationSettings(ctx, rus, canAccessUnit)
		if err != nil {
			annotatedErr := internalerrors.Errorf(
				"updating unit and application settings for %q: %w",
				unitTag.Id(), err)
			return internalerrors.Capture(annotatedErr)
		}
	}

	if len(changes.OpenPorts)+len(changes.ClosePorts) > 0 {
		openPorts := network.GroupedPortRanges{}
		for _, r := range changes.OpenPorts {
			// Ensure the tag in the port open request matches the root unit name
			if r.Tag != changes.Tag {
				return apiservererrors.ErrPerm
			}

			// ICMP is not supported on CAAS models.
			if u.modelType == model.CAAS && r.Protocol == "icmp" {
				return errors.NotSupportedf("protocol icmp on caas models")
			}

			portRange := network.PortRange{
				FromPort: r.FromPort,
				ToPort:   r.ToPort,
				Protocol: r.Protocol,
			}
			openPorts[r.Endpoint] = append(openPorts[r.Endpoint], portRange)
		}

		closePorts := network.GroupedPortRanges{}
		for _, r := range changes.ClosePorts {
			// Ensure the tag in the port close request matches the root unit name
			if r.Tag != changes.Tag {
				return apiservererrors.ErrPerm
			}

			portRange := network.PortRange{
				FromPort: r.FromPort,
				ToPort:   r.ToPort,
				Protocol: r.Protocol,
			}
			closePorts[r.Endpoint] = append(closePorts[r.Endpoint], portRange)
		}

		unitName, err := coreunit.NewName(unitTag.Id())
		if err != nil {
			return internalerrors.Errorf("parsing unit name: %w", err)
		}
		unitUUID, err := u.applicationService.GetUnitUUID(ctx, unitName)
		if err != nil {
			return internalerrors.Errorf("getting UUID of unit %q: %w", unitName, err)
		}
		err = u.portService.UpdateUnitPorts(ctx, unitUUID, openPorts, closePorts)
		if err != nil {
			return internalerrors.Errorf("updating unit ports of unit %q: %w", unitName, err)
		}
	}

	/*
		ctrlCfg, err := u.controllerConfigService.ControllerConfig(ctx)
		if err != nil {
			return errors.Trace(err)
		}
	*/

	if changes.SetUnitState != nil {
		// Ensure the tag in the set state request matches the root unit name
		if changes.SetUnitState.Tag != changes.Tag {
			return apiservererrors.ErrPerm
		}

		unitName, err := coreunit.NewName(unitTag.Id())
		if err != nil {
			return errors.Trace(err)
		}

		// TODO (manadart 2024-10-12): Only charm state is ever set here.
		// The full state is set in the call to SetState (apiserver/common).
		// Integrate this into a transaction with other setters once we are also
		// reading the state from Dqlite.
		// We also need to factor ctrlCfg.MaxCharmStateSize() into the service
		// call.
		if err := u.unitStateService.SetState(ctx, unitstate.UnitState{
			Name:       unitName,
			CharmState: changes.SetUnitState.CharmState,
		}); err != nil {
			return errors.Trace(err)
		}
	}

	for _, addParams := range changes.AddStorage {
		// Ensure the tag in the request matches the root unit name.
		if addParams.UnitTag != changes.Tag {
			return apiservererrors.ErrPerm
		}

		curCons, err := unitStorageConstraints(unitTag)
		if err != nil {
			return errors.Trace(err)
		}

		modelOp, err := u.addStorageToOneUnitOperation(unitTag, addParams, curCons)
		if err != nil {
			return errors.Trace(err)
		}
		modelOps = append(modelOps, modelOp)
	}

	// TODO - do in txn once we have support for that
	if len(changes.SecretCreates) > 0 {
		result, err := u.createSecrets(ctx, params.CreateSecretArgs{Args: changes.SecretCreates})
		if err == nil {
			var errorStrings []string
			for _, r := range result.Results {
				if r.Error != nil {
					errorStrings = append(errorStrings, r.Error.Error())
				}
			}
			if errorStrings != nil {
				err = errors.New(strings.Join(errorStrings, "\n"))
			}
		}
		if err != nil {
			return errors.Annotate(err, "creating secrets")
		}
	}
	if len(changes.SecretUpdates) > 0 {
		result, err := u.updateSecrets(ctx, params.UpdateSecretArgs{Args: changes.SecretUpdates})
		if err == nil {
			err = result.Combine()
		}
		if err != nil {
			return errors.Annotate(err, "updating secrets")
		}
	}
	if len(changes.TrackLatest) > 0 {
		result, err := u.updateTrackedRevisions(ctx, changes.TrackLatest)
		if err == nil {
			err = result.Combine()
		}
		if err != nil {
			return errors.Annotate(err, "updating secret tracked revisions")
		}
	}
	if len(changes.SecretGrants) > 0 {
		result, err := u.secretsGrant(ctx, params.GrantRevokeSecretArgs{Args: changes.SecretGrants})
		if err == nil {
			err = result.Combine()
		}
		if err != nil {
			return errors.Annotate(err, "granting secrets access")
		}
	}
	if len(changes.SecretRevokes) > 0 {
		result, err := u.secretsRevoke(ctx, params.GrantRevokeSecretArgs{Args: changes.SecretRevokes})
		if err == nil {
			err = result.Combine()
		}
		if err != nil {
			return errors.Annotate(err, "revoking secrets access")
		}
	}
	if len(changes.SecretDeletes) > 0 {
		result, err := u.removeSecrets(ctx, params.DeleteSecretArgs{Args: changes.SecretDeletes})
		if err == nil {
			err = result.Combine()
		}
		if err != nil {
			return errors.Annotate(err, "removing secrets")
		}
	}

	// Apply all changes in a single transaction.
	return u.st.ApplyOperation(state.ComposeModelOperations(modelOps...))
}

// WatchInstanceData is a shim to call the LXDProfileAPI version of this method.
func (u *UniterAPI) WatchInstanceData(ctx context.Context, args params.Entities) (params.NotifyWatchResults, error) {
	return u.lxdProfileAPI.WatchInstanceData(ctx, args)
}

// LXDProfileName is a shim to call the LXDProfileAPI version of this method.
func (u *UniterAPI) LXDProfileName(ctx context.Context, args params.Entities) (params.StringResults, error) {
	return u.lxdProfileAPI.LXDProfileName(ctx, args)
}

// LXDProfileRequired is a shim to call the LXDProfileAPI version of this method.
func (u *UniterAPI) LXDProfileRequired(ctx context.Context, args params.CharmURLs) (params.BoolResults, error) {
	return u.lxdProfileAPI.LXDProfileRequired(ctx, args)
}

// CanApplyLXDProfile is a shim to call the LXDProfileAPI version of this method.
func (u *UniterAPI) CanApplyLXDProfile(ctx context.Context, args params.Entities) (params.BoolResults, error) {
	return u.lxdProfileAPI.CanApplyLXDProfile(ctx, args)
}

// WatchApplication starts an NotifyWatcher for an application.
// WatchApplication is not implemented in the UniterAPIv20 facade.
//
// TODO(jack-w-shaw): Replace this with a set of endpoints that watch for specific
// changes to an application. This facade endpoint was added in 21, which has not
// been released yet so we can remove it without worry.
func (u *UniterAPI) WatchApplication(ctx context.Context, entity params.Entity) (params.NotifyWatchResult, error) {
	canWatch, err := u.accessApplication(ctx)
	if err != nil {
		return params.NotifyWatchResult{}, errors.Trace(err)
	}

	tag, err := names.ParseApplicationTag(entity.Tag)
	if err != nil {
		return params.NotifyWatchResult{Error: apiservererrors.ServerError(apiservererrors.ErrPerm)}, nil
	}

	if !canWatch(tag) {
		return params.NotifyWatchResult{Error: apiservererrors.ServerError(apiservererrors.ErrPerm)}, nil
	}

	watcher, err := u.applicationService.WatchApplication(ctx, tag.Id())
	if err != nil {
		return params.NotifyWatchResult{Error: apiservererrors.ServerError(err)}, nil
	}

	id, _, err := internal.EnsureRegisterWatcher[struct{}](ctx, u.watcherRegistry, watcher)
	return params.NotifyWatchResult{
		NotifyWatcherId: id,
		Error:           apiservererrors.ServerError(err),
	}, nil
}

// WatchUnit starts an NotifyWatcher for a unit.
// WatchUnit is not implemented in the UniterAPIv20 facade.
//
// TODO(jack-w-shaw): Remove this ASAP. It was added on facade version 21, which
// has not yet been released. We should not watch for _all_ (how do we define 'all')
// changes to an entity. Instead, we should watch for specific changes.
func (u *UniterAPI) WatchUnit(ctx context.Context, entity params.Entity) (params.NotifyWatchResult, error) {
	canWatch, err := u.accessUnit(ctx)
	if err != nil {
		return params.NotifyWatchResult{}, errors.Trace(err)
	}

	tag, err := names.ParseUnitTag(entity.Tag)
	if err != nil {
		return params.NotifyWatchResult{Error: apiservererrors.ServerError(apiservererrors.ErrPerm)}, nil
	}

	if !canWatch(tag) {
		return params.NotifyWatchResult{Error: apiservererrors.ServerError(apiservererrors.ErrPerm)}, nil
	}

	watcher, err := u.watchUnit(ctx, tag)
	if err != nil {
		return params.NotifyWatchResult{Error: apiservererrors.ServerError(err)}, nil
	}

	id, _, err := internal.EnsureRegisterWatcher[struct{}](ctx, u.watcherRegistry, watcher)
	return params.NotifyWatchResult{
		NotifyWatcherId: id,
		Error:           apiservererrors.ServerError(err),
	}, nil
}

// Watch starts an NotifyWatcher for a unit or application.
// This is being deprecated in favour of separate WatchUnit and WatchApplication
// methods.
func (u *UniterAPIv20) Watch(ctx context.Context, args params.Entities) (params.NotifyWatchResults, error) {
	result := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	canWatch, err := u.accessUnitOrApplication(ctx)
	if err != nil {
		return params.NotifyWatchResults{}, errors.Trace(err)
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}

		if !canWatch(tag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}

		var watcher watcher.NotifyWatcher
		switch t := tag.(type) {
		case names.ApplicationTag:
			watcher, err = u.applicationService.WatchApplication(ctx, t.Id())
		case names.UnitTag:
			watcher, err = u.watchUnit(ctx, t)
		default:
			result.Results[i].Error = apiservererrors.ServerError(errors.NotSupportedf("tag type %T", tag))
			continue
		}
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		id, _, err := internal.EnsureRegisterWatcher[struct{}](ctx, u.watcherRegistry, watcher)
		result.Results[i].NotifyWatcherId = id
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

// watchUnit returns a state notify watcher for the given unit.
func (u *UniterAPI) watchUnit(ctx context.Context, tag names.UnitTag) (watcher.NotifyWatcher, error) {
	unitName, err := coreunit.NewName(tag.Id())
	if err != nil {
		return nil, internalerrors.Errorf("parsing unit name: %w", err)
	}

	watcher, err := u.applicationService.WatchUnitForLegacyUniter(ctx, unitName)
	if errors.Is(err, applicationerrors.UnitNotFound) {
		return nil, errors.NotFoundf("unit %q", unitName)
	} else if err != nil {
		return nil, internalerrors.Errorf("watching unit %q: %w", unitName, err)
	}
	return watcher, nil
}

// Merge merges in the provided leadership settings. Only leaders for
// the given service may perform this operation.
func (u *UniterAPIv20) Merge(ctx context.Context, bulkArgs params.MergeLeadershipSettingsBulkParams) (params.ErrorResults, error) {
	results := make([]params.ErrorResult, len(bulkArgs.Params))
	return params.ErrorResults{Results: results}, nil
}

// Read reads leadership settings for the provided service ID. Any
// unit of the service may perform this operation.
func (u *UniterAPIv20) Read(ctx context.Context, bulkArgs params.Entities) (params.GetLeadershipSettingsBulkResults, error) {
	results := make([]params.GetLeadershipSettingsResult, len(bulkArgs.Entities))
	return params.GetLeadershipSettingsBulkResults{Results: results}, nil
}

// WatchLeadershipSettings will block the caller until leadership settings
// for the given service ID change.
func (u *UniterAPIv20) WatchLeadershipSettings(ctx context.Context, bulkArgs params.Entities) (params.NotifyWatchResults, error) {
	results := make([]params.NotifyWatchResult, len(bulkArgs.Entities))

	for i := range bulkArgs.Entities {
		result := &results[i]

		// We need a notify watcher for each item, otherwise during a migration
		// a 3.x agent will bounce and will not be able to continue. By
		// providing a watcher which does nothing, we can ensure that the 3.x
		// agent will continue to work.
		watcher := watcher.TODO[struct{}]()
		id, _, err := internal.EnsureRegisterWatcher(ctx, u.watcherRegistry, watcher)
		if err != nil {
			result.Error = apiservererrors.ServerError(err)
			continue
		}
		result.NotifyWatcherId = id
	}
	return params.NotifyWatchResults{Results: results}, nil
}

// Merge is not implemented in version 21 of the uniter.
func (u *UniterAPI) Merge(ctx context.Context, _, _ struct{}) {}

// Read is not implemented in version 21 of the uniter.
func (u *UniterAPI) Read(ctx context.Context, _, _ struct{}) {}

// WatchLeadershipSettings is not implemented in version 21 of the uniter.
func (u *UniterAPI) WatchLeadershipSettings(ctx context.Context, _, _ struct{}) {}

func ptr[T any](v T) *T {
	return &v
}
