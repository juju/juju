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
	"github.com/juju/juju/apiserver/common/cloudspec"
	commonmodel "github.com/juju/juju/apiserver/common/model"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	apiservercharms "github.com/juju/juju/apiserver/internal/charms"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/leadership"
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
	applicationerrors "github.com/juju/juju/domain/application/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/domain/relation"
	"github.com/juju/juju/domain/unitstate"
	"github.com/juju/juju/internal/charm"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
	statewatcher "github.com/juju/juju/state/watcher"
)

// UniterAPI implements the latest version (v21) of the Uniter API.
type UniterAPI struct {
	*StatusAPI
	*StorageAPI

	*common.APIAddresser
	*commonmodel.ModelConfigWatcher
	*common.RebootRequester
	*common.UnitStateAPI

	lxdProfileAPI            *LXDProfileAPIv2
	environConfigGetterModel EnvironConfigGetterModel
	st                       *state.State
	clock                    clock.Clock
	auth                     facade.Authorizer
	resources                facade.Resources
	leadershipChecker        leadership.Checker
	leadershipRevoker        leadership.Revoker
	accessUnit               common.GetAuthFunc
	accessApplication        common.GetAuthFunc
	accessUnitOrApplication  common.GetAuthFunc
	accessMachine            common.GetAuthFunc
	containerBrokerFunc      caas.NewContainerBrokerFunc
	watcherRegistry          facade.WatcherRegistry

	applicationService      ApplicationService
	cloudService            CloudService
	controllerConfigService ControllerConfigService
	credentialService       CredentialService
	machineService          MachineService
	modelConfigService      ModelConfigService
	modelInfoService        ModelInfoService
	networkService          NetworkService
	portService             PortService
	relationService         RelationService
	secretService           SecretService
	unitStateService        UnitStateService

	store objectstore.ObjectStore

	// A cloud spec can only be accessed for the model of the unit or
	// application that is authorised for this API facade.
	// We do not need to use an AuthFunc, because we do not need to pass a tag.
	accessCloudSpec func() (func() bool, error)
	cloudSpecer     cloudspec.CloudSpecer

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
	canModify, err := u.accessUnit()
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

		// TODO(units) - remove me.
		// Dual write dead status to state.
		unit, err := u.getUnit(tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if err = unit.EnsureDead(); err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
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
	canAccess, err := u.accessMachine()
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
	machineOpenedPortRanges, err := u.portService.GetMachineOpenedPorts(ctx, machineUUID)
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
	canAccess, err := u.accessUnit()
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
		unit, err := u.getUnit(tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		machineId, err := unit.AssignedMachineId()
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
		} else {
			result.Results[i].Result = names.NewMachineTag(machineId).String()
		}
	}
	return result, nil
}

// PublicAddress returns the public address for each given unit, if set.
func (u *UniterAPI) PublicAddress(ctx context.Context, args params.Entities) (params.StringResults, error) {
	result := params.StringResults{
		Results: make([]params.StringResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.StringResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		err = apiservererrors.ErrPerm
		if canAccess(tag) {
			var unit *state.Unit
			unit, err = u.getUnit(tag)
			if err == nil {
				var address network.SpaceAddress
				address, err = unit.PublicAddress()
				if err == nil {
					result.Results[i].Result = address.Value
				} else if network.IsNoAddressError(err) {
					err = apiservererrors.NewNoAddressSetError(tag, "public")
				}
			}
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

// PrivateAddress returns the private address for each given unit, if set.
func (u *UniterAPI) PrivateAddress(ctx context.Context, args params.Entities) (params.StringResults, error) {
	result := params.StringResults{
		Results: make([]params.StringResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.StringResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		err = apiservererrors.ErrPerm
		if canAccess(tag) {
			var unit *state.Unit
			unit, err = u.getUnit(tag)
			if err == nil {
				var address network.SpaceAddress
				address, err = unit.PrivateAddress()
				if err == nil {
					result.Results[i].Result = address.Value
				} else if network.IsNoAddressError(err) {
					err = apiservererrors.NewNoAddressSetError(tag, "private")
				}
			}
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

// TODO(ericsnow) Factor out the common code amongst the many methods here.

var getZone = func(ctx context.Context, st *state.State, machineService MachineService, tag names.Tag) (string, error) {
	unit, err := st.Unit(tag.Id())
	if err != nil {
		return "", errors.Trace(err)
	}
	machineID, err := unit.AssignedMachineId()
	if err != nil {
		return "", errors.Trace(err)
	}
	machineUUID, err := machineService.GetMachineUUID(ctx, coremachine.Name(machineID))
	if err != nil {
		return "", errors.Trace(err)
	}
	az, err := machineService.AvailabilityZone(ctx, machineUUID)
	if errors.Is(err, machineerrors.AvailabilityZoneNotFound) {
		return "", errors.NotProvisioned
	}
	if err != nil {
		return "", errors.Trace(err)
	}
	return az, errors.Trace(err)
}

// AvailabilityZone returns the availability zone for each given unit, if applicable.
func (u *UniterAPI) AvailabilityZone(ctx context.Context, args params.Entities) (params.StringResults, error) {
	var results params.StringResults

	canAccess, err := u.accessUnit()
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
		err = apiservererrors.ErrPerm
		if canAccess(tag) {
			var zone string
			zone, err = getZone(ctx, u.st, u.machineService, tag)
			if err == nil {
				results.Results[i].Result = zone
			}
		}
		results.Results[i].Error = apiservererrors.ServerError(err)
	}

	return results, nil
}

// Resolved returns the current resolved setting for each given unit.
func (u *UniterAPI) Resolved(ctx context.Context, args params.Entities) (params.ResolvedModeResults, error) {
	result := params.ResolvedModeResults{
		Results: make([]params.ResolvedModeResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.ResolvedModeResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		err = apiservererrors.ErrPerm
		if canAccess(tag) {
			var unit *state.Unit
			unit, err = u.getUnit(tag)
			if err == nil {
				result.Results[i].Mode = params.ResolvedMode(unit.Resolved())
			}
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

// ClearResolved removes any resolved setting from each given unit.
func (u *UniterAPI) ClearResolved(ctx context.Context, args params.Entities) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit()
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
			var unit *state.Unit
			unit, err = u.getUnit(tag)
			if err == nil {
				err = unit.ClearResolved()
			}
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

// GetPrincipal returns the result of calling PrincipalName() and
// converting it to a tag, on each given unit.
func (u *UniterAPI) GetPrincipal(ctx context.Context, args params.Entities) (params.StringBoolResults, error) {
	result := params.StringBoolResults{
		Results: make([]params.StringBoolResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.StringBoolResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		err = apiservererrors.ErrPerm
		if canAccess(tag) {
			var unit *state.Unit
			unit, err = u.getUnit(tag)
			if err == nil {
				principal, ok := unit.PrincipalName()
				if principal != "" {
					result.Results[i].Result = names.NewUnitTag(principal).String()
				}
				result.Results[i].Ok = ok
			}
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

// Destroy advances all given Alive units' lifecycles as far as
// possible. See state/Unit.Destroy().
func (u *UniterAPI) Destroy(ctx context.Context, args params.Entities) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit()
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
			unit, err = u.getUnit(tag)
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
	canAccess, err := u.accessUnit()
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
			var unit *state.Unit
			unit, err = u.getUnit(tag)
			if err == nil {
				err = u.destroySubordinates(ctx, unit)
			}
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

// HasSubordinates returns the whether each given unit has any subordinates.
func (u *UniterAPI) HasSubordinates(ctx context.Context, args params.Entities) (params.BoolResults, error) {
	result := params.BoolResults{
		Results: make([]params.BoolResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.BoolResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		err = apiservererrors.ErrPerm
		if canAccess(tag) {
			var unit *state.Unit
			unit, err = u.getUnit(tag)
			if err == nil {
				subordinates := unit.SubordinateNames()
				result.Results[i].Result = len(subordinates) > 0
			}
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
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
	canAccess, err := accessUnitOrApplication()
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
func (u *UniterAPI) CharmURL(ctx context.Context, args params.Entities) (params.StringBoolResults, error) {
	result := params.StringBoolResults{
		Results: make([]params.StringBoolResult, len(args.Entities)),
	}
	accessUnitOrApplication := common.AuthAny(u.accessUnit, u.accessApplication)
	canAccess, err := accessUnitOrApplication()
	if err != nil {
		return params.StringBoolResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		err = apiservererrors.ErrPerm
		if canAccess(tag) {
			var unitOrApplication state.Entity
			unitOrApplication, err = u.st.FindEntity(tag)
			if err == nil {
				var cURL *string
				var force bool

				switch entity := unitOrApplication.(type) {
				case *state.Application:
					cURL, force = entity.CharmURL()
				case *state.Unit:
					cURL = entity.CharmURL()
					// The force value is not actually used on the uniter's unit api.
					if cURL != nil {
						force = true
					}
				}

				if cURL != nil {
					result.Results[i].Result = *cURL
					result.Results[i].Ok = force
				}
			}
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

// SetCharmURL sets the charm URL for each given unit. An error will
// be returned if a unit is dead, or the charm URL is not known.
func (u *UniterAPI) SetCharmURL(ctx context.Context, args params.EntitiesCharmURL) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit()
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
			var unit *state.Unit
			unit, err = u.getUnit(tag)
			if err == nil {
				err = unit.SetCharmURL(entity.CharmURL)
			}
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
	canAccess, err := u.accessUnit()
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
		unit, err := u.getUnit(tag)
		if err != nil {
			resultItem.Error = apiservererrors.ServerError(err)
			continue
		}
		version, err := unit.WorkloadVersion()
		if err != nil {
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
	canAccess, err := u.accessUnit()
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
		unit, err := u.getUnit(tag)
		if err != nil {
			resultItem.Error = apiservererrors.ServerError(err)
			continue
		}
		err = unit.SetWorkloadVersion(entity.WorkloadVersion)
		if err != nil {
			resultItem.Error = apiservererrors.ServerError(err)
		}
	}
	return result, nil
}

// WatchActionNotifications returns a StringsWatcher for observing
// incoming action calls to a unit. See also state/watcher.go
// Unit.WatchActionNotifications(). This method is called from
// api/uniter/uniter.go WatchActionNotifications().
func (u *UniterAPI) WatchActionNotifications(ctx context.Context, args params.Entities) (params.StringsWatchResults, error) {
	tagToActionReceiver := common.TagToActionReceiverFn(u.st.FindEntity)
	watchOne := common.WatchOneActionReceiverNotifications(tagToActionReceiver, u.resources.Register)
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.StringsWatchResults{}, err
	}
	return common.WatchActionNotifications(args, canAccess, watchOne), nil
}

// ConfigSettings returns the complete set of application charm config
// settings available to each given unit.
func (u *UniterAPI) ConfigSettings(ctx context.Context, args params.Entities) (params.ConfigSettingsResults, error) {
	result := params.ConfigSettingsResults{
		Results: make([]params.ConfigSettingsResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.ConfigSettingsResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		err = apiservererrors.ErrPerm
		if canAccess(tag) {
			var unit *state.Unit
			unit, err = u.st.Unit(tag.Id())
			if errors.Is(err, errors.NotFound) {
				result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
				continue
			}
			if err != nil {
				result.Results[i].Error = apiservererrors.ServerError(err)
				continue
			}

			var settings charm.Settings
			settings, err = unit.ConfigSettings()
			if err == nil {
				result.Results[i].Settings = params.ConfigSettings(settings)
			}
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
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
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.RelationResults{}, err
	}
	modelInfo, err := u.modelInfoService.GetModelInfo(ctx)
	if err != nil {
		return result, err
	}
	for i, rel := range args.RelationUnits {
		relParams, err := u.getOneRelation(ctx, canAccess, rel.Relation, rel.Unit, modelInfo.UUID)
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
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.RelationResultsV2{}, err
	}
	modelInfo, err := u.modelInfoService.GetModelInfo(ctx)
	if err != nil {
		return result, err
	}
	for i, rel := range args.RelationUnits {
		relParams, err := u.getOneRelation(ctx, canAccess, rel.Relation, rel.Unit, modelInfo.UUID)
		if err == nil {
			result.Results[i] = relParams
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

// ActionStatus returns the status of Actions by Tags passed in.
func (u *UniterAPI) ActionStatus(ctx context.Context, args params.Entities) (params.StringResults, error) {
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.StringResults{}, err
	}

	m, err := u.st.Model()
	if err != nil {
		return params.StringResults{}, errors.Trace(err)
	}

	results := params.StringResults{
		Results: make([]params.StringResult, len(args.Entities)),
	}

	actionFn := common.AuthAndActionFromTagFn(canAccess, m.ActionByTag)
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
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.ActionResults{}, err
	}

	m, err := u.st.Model()
	if err != nil {
		return params.ActionResults{}, errors.Trace(err)
	}

	actionFn := common.AuthAndActionFromTagFn(canAccess, m.ActionByTag)
	return common.Actions(args, actionFn), nil
}

// BeginActions marks the actions represented by the passed in Tags as running.
func (u *UniterAPI) BeginActions(ctx context.Context, args params.Entities) (params.ErrorResults, error) {
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.ErrorResults{}, err
	}

	m, err := u.st.Model()
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}

	actionFn := common.AuthAndActionFromTagFn(canAccess, m.ActionByTag)
	return common.BeginActions(args, actionFn), nil
}

// FinishActions saves the result of a completed Action
func (u *UniterAPI) FinishActions(ctx context.Context, args params.ActionExecutionResults) (params.ErrorResults, error) {
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.ErrorResults{}, err
	}

	m, err := u.st.Model()
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}

	actionFn := common.AuthAndActionFromTagFn(canAccess, m.ActionByTag)
	return common.FinishActions(args, actionFn), nil
}

// LogActionsMessages records the log messages against the specified actions.
func (u *UniterAPI) LogActionsMessages(ctx context.Context, args params.ActionMessageParams) (params.ErrorResults, error) {
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.ErrorResults{}, err
	}
	m, err := u.st.Model()
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	actionFn := common.AuthAndActionFromTagFn(canAccess, m.ActionByTag)

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
	modelInfo, err := u.modelInfoService.GetModelInfo(ctx)
	if err != nil {
		return result, err
	}
	for i, relId := range args.RelationIds {
		relParams, err := u.getOneRelationById(ctx, relId, modelInfo.UUID)
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
	modelInfo, err := u.modelInfoService.GetModelInfo(ctx)
	if err != nil {
		return result, err
	}
	for i, relId := range args.RelationIds {
		relParams, err := u.getOneRelationById(ctx, relId, modelInfo.UUID)
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
	canRead, err := u.accessUnit()
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
	canRead, err := u.accessUnitOrApplication()
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
			lifeValue, err = u.applicationService.GetApplicationLife(ctx, tag.Id())
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
func (u *UniterAPI) Refresh(ctx context.Context, args params.Entities) (params.UnitRefreshResults, error) {
	result := params.UnitRefreshResults{
		Results: make([]params.UnitRefreshResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	canRead, err := u.accessUnit()
	if err != nil {
		return params.UnitRefreshResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		err = apiservererrors.ErrPerm
		if canRead(tag) {
			// TODO(units) - read unit details from dqlite
			var unit *state.Unit
			if unit, err = u.getUnit(tag); err == nil {
				result.Results[i].Life = life.Value(unit.Life().String())
				result.Results[i].Resolved = params.ResolvedMode(unit.Resolved())

				var err1 error
				result.Results[i].ProviderID, err1 = u.getProviderID(unit)
				if err1 != nil && !errors.Is(err1, errors.NotFound) {
					// initially, it returns not found error, so just ignore it.
					err = err1
				}
			}
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func (u *UniterAPI) getProviderID(unit *state.Unit) (string, error) {
	container, err := unit.ContainerInfo()
	if err != nil {
		return "", err
	}
	return container.ProviderId(), nil
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
// ModelConfig() just to get the provider type. Once we have machine
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
// for all of the given relation/unit pairs. See also
// state.RelationUnit.EnterScope().
func (u *UniterAPI) EnterScope(ctx context.Context, args params.RelationUnits) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.RelationUnits)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.ErrorResults{}, err
	}
	one := func(relTag string, unitTag names.UnitTag) error {
		rel, unit, err := u.getRelationAndUnit(canAccess, relTag, unitTag)
		if err != nil {
			return err
		}
		relUnit, err := rel.Unit(unit)
		if err != nil {
			return err
		}

		valid, err := relUnit.Valid()
		if err != nil {
			return err
		}
		if !valid {
			principalName, _ := unit.PrincipalName()
			u.logger.Debugf(context.TODO(), "ignoring %q EnterScope for %q - unit has invalid principal %q",
				unit.Name(), rel.String(), principalName)
			return nil
		}

		netInfo, err := NewNetworkInfo(ctx, u.st, u.clock, u.networkService, u.modelConfigService, unitTag, u.logger)
		if err != nil {
			return err
		}

		settings := map[string]interface{}{}
		_, ingressAddresses, egressSubnets, err := netInfo.NetworksForRelation(relUnit.Endpoint().Name, rel)
		if err == nil && len(ingressAddresses) > 0 {
			ingressAddress := ingressAddresses[0].Value
			// private-address is historically a cloud local address for the machine.
			// Existing charms are built to ask for this attribute from relation
			// settings to find out what address to use to connect to the app
			// on the other side of a relation. For cross model scenarios, we'll
			// replace this with possibly a public address; we expect to fix more
			// charms than we break - breakage will not occur for correctly written
			// charms, since the semantics of this value dictates the use case described.
			// Any other use goes against the intended purpose of this value.
			settings["private-address"] = ingressAddress
			// ingress-address is the preferred settings attribute name as it more accurately
			// reflects the purpose of the attribute value. We'll deprecate private-address.
			settings["ingress-address"] = ingressAddress
		} else if err != nil {
			u.logger.Warningf(context.TODO(), "cannot set ingress/egress addresses for unit %v in relation %v: %v",
				unitTag.Id(), relTag, err)
		}
		if len(egressSubnets) > 0 {
			settings["egress-subnets"] = strings.Join(egressSubnets, ",")
		}
		return relUnit.EnterScope(settings)
	}
	for i, arg := range args.RelationUnits {
		tag, err := names.ParseUnitTag(arg.Unit)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		err = one(arg.Relation, tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
		}
	}
	return result, nil
}

// LeaveScope signals each unit has left its scope in the relation,
// for all of the given relation/unit pairs. See also
// state.RelationUnit.LeaveScope().
func (u *UniterAPI) LeaveScope(ctx context.Context, args params.RelationUnits) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.RelationUnits)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.ErrorResults{}, err
	}
	for i, arg := range args.RelationUnits {
		unit, err := names.ParseUnitTag(arg.Unit)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		relUnit, err := u.getRelationUnit(canAccess, arg.Relation, unit)
		if err == nil {
			err = relUnit.LeaveScope()
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
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
	canAccessUnit, err := u.accessUnit()
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

func (u UniterAPI) readOneUnitSettings(
	ctx context.Context,
	canAccessUnit common.AuthFunc,
	arg params.RelationUnit,
) (params.Settings, error) {
	unitTag, err := names.ParseTag(arg.Unit)
	if err != nil {
		return nil, apiservererrors.ErrPerm
	}
	relTag, err := names.ParseRelationTag(arg.Relation)
	if err != nil {
		return nil, apiservererrors.ErrPerm
	}
	relUUID, err := u.relationService.GetRelationUUIDFromKey(ctx, corerelation.Key(relTag.Id()))
	if errors.Is(err, errors.NotFound) {
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
	relTag, err := names.ParseRelationTag(arg.Relation)
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

	relUUID, err := u.relationService.GetRelationUUIDFromKey(ctx, corerelation.Key(relTag.Id()))
	if errors.Is(err, errors.NotFound) {
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
	canAccessApp, err := u.accessApplication()
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

	settings, err := u.relationService.GetLocalRelationApplicationSettings(ctx, unitName, relUUID, appID)
	if errors.Is(err, errors.NotFound) {
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
	canAccess, err := u.accessUnit()
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

	relationTag, err := names.ParseTag(arg.Relation)
	if err != nil {
		return nil, apiservererrors.ErrPerm
	}

	relUUID, err := u.relationService.GetRelationUUIDFromKey(ctx, corerelation.Key(relationTag.Id()))
	if err != nil {
		return nil, err
	}

	var settings map[string]string

	switch tag := remoteTag.(type) {
	case names.UnitTag:
		unitName, err := coreunit.NewName(tag.Id())
		if err != nil {
			return nil, internalerrors.Capture(err)
		}
		relUnitUUID, err := u.relationService.GetRelationUnit(ctx, relUUID, unitName)
		if err != nil {
			return nil, internalerrors.Capture(err)
		}
		settings, err = u.relationService.GetRelationUnitSettings(ctx, relUnitUUID)
		if err != nil {
			return nil, internalerrors.Capture(err)
		}
	case names.ApplicationTag:
		appID, err := u.applicationService.GetApplicationIDByName(ctx, tag.Id())
		if err != nil {
			return nil, internalerrors.Capture(err)
		}
		settings, err = u.relationService.GetRemoteRelationApplicationSettings(ctx, relUUID, appID)
		if err != nil {
			return nil, internalerrors.Capture(err)
		}
	default:
		return nil, apiservererrors.ErrPerm
	}
	return settings, nil
}

func (u *UniterAPI) updateUnitAndApplicationSettingsOp(arg params.RelationUnitSettings, canAccess common.AuthFunc) (state.ModelOperation, error) {
	unitTag, err := names.ParseUnitTag(arg.Unit)
	if err != nil {
		return nil, apiservererrors.ErrPerm
	}
	rel, unit, err := u.getRelationAndUnit(canAccess, arg.Relation, unitTag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	relUnit, err := rel.Unit(unit)
	if err != nil {
		return nil, errors.Trace(err)
	}
	appSettingsUpdateOp, err := u.updateApplicationSettingsOp(rel, unit, arg.ApplicationSettings)
	if err != nil {
		return nil, errors.Trace(err)
	}
	unitSettingsUpdateOp, err := u.updateUnitSettingsOp(relUnit, arg.Settings)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return state.ComposeModelOperations(appSettingsUpdateOp, unitSettingsUpdateOp), nil
}

func (u *UniterAPI) updateUnitSettingsOp(relUnit *state.RelationUnit, newSettings params.Settings) (state.ModelOperation, error) {
	if len(newSettings) == 0 {
		return nil, nil
	}
	settings, err := relUnit.Settings()
	if err != nil {
		return nil, errors.Trace(err)
	}
	for k, v := range newSettings {
		if v == "" {
			settings.Delete(k)
		} else {
			settings.Set(k, v)
		}
	}
	return settings.WriteOperation(), nil
}

func (u *UniterAPI) updateApplicationSettingsOp(rel *state.Relation, unit *state.Unit, settings params.Settings) (state.ModelOperation, error) {
	if len(settings) == 0 {
		return nil, nil
	}
	token := u.leadershipChecker.LeadershipCheck(unit.ApplicationName(), unit.Name())
	settingsMap := make(map[string]interface{}, len(settings))
	for k, v := range settings {
		settingsMap[k] = v
	}

	return rel.UpdateApplicationSettingsOperation(unit.ApplicationName(), token, settingsMap)
}

// WatchRelationUnits returns a RelationUnitsWatcher for observing
// changes to every unit in the supplied relation that is visible to
// the supplied unit. See also state/watcher.go:RelationUnit.Watch().
func (u *UniterAPI) WatchRelationUnits(ctx context.Context, args params.RelationUnits) (params.RelationUnitsWatchResults, error) {
	result := params.RelationUnitsWatchResults{
		Results: make([]params.RelationUnitsWatchResult, len(args.RelationUnits)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.RelationUnitsWatchResults{}, err
	}
	for i, arg := range args.RelationUnits {
		unit, err := names.ParseUnitTag(arg.Unit)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		relUnit, err := u.getRelationUnit(canAccess, arg.Relation, unit)
		if err == nil {
			result.Results[i], err = u.watchOneRelationUnit(relUnit)
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
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
	if errors.Is(err, errors.NotFound) {
		return apiservererrors.ErrPerm
	} else if err != nil {
		return internalerrors.Capture(err)
	}

	// If we are transitioning from "suspending" to "suspended",
	// we retain any existing message so that if the user has
	// previously specified a reason for suspending, it is retained.
	if message == "" && relStatus == params.Suspended {
		current, err := u.relationService.GetRelationStatus(ctx, relationUUID)
		if err != nil {
			return internalerrors.Capture(err)
		}
		if current.Status == status.Suspending {
			message = current.Message
		}
	}
	err = u.relationService.SetRelationStatus(ctx, unitName, relationUUID, status.StatusInfo{
		Status:  status.Status(relStatus),
		Message: message,
	})
	if errors.Is(err, errors.NotFound) {
		return apiservererrors.ErrPerm
	} else if err != nil {
		return internalerrors.Capture(err)
	}
	return nil
}

func (u *UniterAPI) getUnit(tag names.UnitTag) (*state.Unit, error) {
	return u.st.Unit(tag.Id())
}

func (u *UniterAPI) getRelationUnit(canAccess common.AuthFunc, relTag string, unitTag names.UnitTag) (*state.RelationUnit, error) {
	rel, unit, err := u.getRelationAndUnit(canAccess, relTag, unitTag)
	if err != nil {
		return nil, err
	}
	return rel.Unit(unit)
}

func (u *UniterAPI) getOneRelationById(ctx context.Context, relID int, modelUUID model.UUID) (params.RelationResultV2, error) {
	nothing := params.RelationResultV2{}
	rel, err := u.relationService.GetRelationDetails(ctx, relID)
	if errors.Is(err, errors.NotFound) {
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
	result, err := u.prepareRelationResult(rel, applicationName, modelUUID)
	if err != nil {
		// An error from prepareRelationResult means the authenticated
		// unit's application is not part of the requested
		// relation. That's why it's appropriate to return ErrPerm
		// here.
		return nothing, apiservererrors.ErrPerm
	}
	return result, nil
}

func (u *UniterAPI) getRelation(relTag string) (*state.Relation, error) {
	tag, err := names.ParseRelationTag(relTag)
	if err != nil {
		return nil, apiservererrors.ErrPerm
	}
	rel, err := u.st.KeyRelation(tag.Id())
	if errors.Is(err, errors.NotFound) {
		return nil, apiservererrors.ErrPerm
	} else if err != nil {
		return nil, err
	}
	return rel, nil
}

func (u *UniterAPI) getRelationAndUnit(canAccess common.AuthFunc, relTag string, unitTag names.UnitTag) (*state.Relation, *state.Unit, error) {
	rel, err := u.getRelation(relTag)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	if !canAccess(unitTag) {
		return nil, nil, apiservererrors.ErrPerm
	}
	unit, err := u.getUnit(unitTag)
	return rel, unit, err
}

func (u *UniterAPI) prepareRelationResult(
	rel relation.RelationDetails,
	applicationName string,
	modelUUID model.UUID,
) (params.RelationResultV2, error) {
	var (
		otherAppName string
		unitEp       relation.Endpoint
	)
	for _, v := range rel.Endpoint {
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
		ModelUUID:       modelUUID.String(),
	}

	return params.RelationResultV2{
		Id:   rel.ID,
		Key:  rel.Key,
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
	modelUUID model.UUID,
) (params.RelationResultV2, error) {
	nothing := params.RelationResultV2{}
	unitTag, err := names.ParseUnitTag(unitTagStr)
	if err != nil {
		return nothing, apiservererrors.ErrPerm
	}
	if !canAccess(unitTag) {
		return nothing, apiservererrors.ErrPerm
	}
	relTag, err := names.ParseRelationTag(relTagStr)
	if err != nil {
		return nothing, apiservererrors.ErrPerm
	}
	relUUID, err := u.relationService.GetRelationUUIDFromKey(ctx, corerelation.Key(relTag.Id()))
	if errors.Is(err, errors.NotFound) {
		return nothing, apiservererrors.ErrPerm
	} else if err != nil {
		return nothing, err
	}
	unitName, err := coreunit.NewName(unitTag.Id())
	if err != nil {
		return nothing, internalerrors.Capture(err)
	}
	rel, err := u.relationService.GetRelationDetailsForUnit(ctx, relUUID, unitName)
	if errors.Is(err, errors.NotFound) {
		return nothing, apiservererrors.ErrPerm
	} else if err != nil {
		return nothing, err
	}
	appName, err := names.UnitApplication(unitTag.Id())
	if err != nil {
		return nothing, apiservererrors.ErrBadId
	}
	return u.prepareRelationResult(rel, appName, modelUUID)
}

func (u *UniterAPI) destroySubordinates(ctx context.Context, principal *state.Unit) error {
	subordinates := principal.SubordinateNames()
	for _, sub := range subordinates {
		subName, err := coreunit.NewName(sub)
		if err != nil {
			return err
		}
		err = u.applicationService.DestroyUnit(ctx, subName)
		if err != nil && !errors.Is(err, applicationerrors.UnitNotFound) {
			return err
		}

		// TODO(units) - remove dual write to state
		unit, err := u.getUnit(names.NewUnitTag(subName.String()))
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

func (u *UniterAPI) watchOneRelationUnit(relUnit *state.RelationUnit) (params.RelationUnitsWatchResult, error) {
	stateWatcher := relUnit.Watch()
	watch, err := common.RelationUnitsWatcherFromState(stateWatcher)
	if err != nil {
		return params.RelationUnitsWatchResult{}, errors.Trace(err)
	}
	// Consume the initial event and forward it to the result.
	if changes, ok := <-watch.Changes(); ok {
		return params.RelationUnitsWatchResult{
			RelationUnitsWatcherId: u.resources.Register(watch),
			Changes:                changes,
		}, nil
	}
	return params.RelationUnitsWatchResult{}, statewatcher.EnsureErr(watch)
}

// NetworkInfo returns network interfaces/addresses for specified bindings.
func (u *UniterAPI) NetworkInfo(ctx context.Context, args params.NetworkInfoParams) (params.NetworkInfoResults, error) {
	canAccess, err := u.accessUnit()
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

	netInfo, err := NewNetworkInfo(ctx, u.st, u.clock, u.networkService, u.modelConfigService, unitTag, u.logger)
	if err != nil {
		return params.NetworkInfoResults{}, err
	}

	res, err := netInfo.ProcessAPIRequest(args)
	if err != nil {
		return params.NetworkInfoResults{}, err
	}
	return uniqueNetworkInfoResults(res), nil
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
	canAccess, err := u.accessUnit()
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
			result.Results[i], err = u.watchOneUnitRelations(tag)
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func (u *UniterAPI) watchOneUnitRelations(tag names.UnitTag) (params.StringsWatchResult, error) {
	nothing := params.StringsWatchResult{}
	unit, err := u.getUnit(tag)
	if err != nil {
		return nothing, err
	}
	app, err := unit.Application()
	if err != nil {
		return nothing, err
	}
	principalName, isSubordinate := unit.PrincipalName()
	var watch state.StringsWatcher
	if isSubordinate {
		principalUnit, err := u.st.Unit(principalName)
		if err != nil {
			return nothing, errors.Trace(err)
		}
		principalApp, err := principalUnit.Application()
		if err != nil {
			return nothing, errors.Trace(err)
		}
		watch, err = newSubordinateRelationsWatcher(u.st, app, principalApp.Name(), u.logger)
		if err != nil {
			return nothing, errors.Trace(err)
		}
	} else {
		watch = app.WatchRelations()
	}
	// Consume the initial event and forward it to the result.
	if changes, ok := <-watch.Changes(); ok {
		return params.StringsWatchResult{
			StringsWatcherId: u.resources.Register(watch),
			Changes:          changes,
		}, nil
	}
	return nothing, statewatcher.EnsureErr(watch)
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
// authenticated unit or application resides.
// A check is made beforehand to ensure that the request is made by an entity
// that has been granted the appropriate trust.
func (u *UniterAPI) CloudSpec(ctx context.Context) (params.CloudSpecResult, error) {
	canAccess, err := u.accessCloudSpec()
	if err != nil {
		return params.CloudSpecResult{}, err
	}
	if !canAccess() {
		return params.CloudSpecResult{Error: apiservererrors.ServerError(apiservererrors.ErrPerm)}, nil
	}

	modelInfo, err := u.modelInfoService.GetModelInfo(ctx)
	if err != nil {
		return params.CloudSpecResult{}, err
	}
	modelTag := names.NewModelTag(modelInfo.UUID.String())
	return u.cloudSpecer.GetCloudSpec(ctx, modelTag), nil
}

// GoalStates returns information of charm units and relations.
func (u *UniterAPI) GoalStates(ctx context.Context, args params.Entities) (params.GoalStateResults, error) {
	result := params.GoalStateResults{
		Results: make([]params.GoalStateResult, len(args.Entities)),
	}

	canAccess, err := u.accessUnit()
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
		unit, err := u.getUnit(tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		result.Results[i].Result, err = u.oneGoalState(ctx, unit)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
		}
	}
	return result, nil
}

// oneGoalState creates the goal state for a given unit.
func (u *UniterAPI) oneGoalState(ctx context.Context, unit *state.Unit) (*params.GoalState, error) {
	app, err := unit.Application()
	if err != nil {
		return nil, err
	}

	gs := params.GoalState{}
	gs.Units, err = u.goalStateUnits(ctx, app, unit.Name())
	if err != nil {
		return nil, err
	}
	allRelations, err := app.Relations()
	if err != nil {
		return nil, err
	}
	if allRelations != nil {
		gs.Relations, err = u.goalStateRelations(ctx, app.Name(), unit.Name(), allRelations)
		if err != nil {
			return nil, err
		}
	}
	return &gs, nil
}

// goalStateRelations creates the structure with all the relations between endpoints in an application.
func (u *UniterAPI) goalStateRelations(ctx context.Context, appName, principalName string, allRelations []*state.Relation) (map[string]params.UnitsGoalState, error) {

	result := map[string]params.UnitsGoalState{}

	for _, r := range allRelations {
		statusInfo, err := r.Status()
		if err != nil {
			return nil, errors.Annotate(err, "getting relation status")
		}
		endPoints := r.Endpoints()
		if len(endPoints) == 1 {
			// Ignore peer relations here.
			continue
		}

		// First determine the local endpoint name to use later
		// as the key in the result map.
		var resultEndpointName string
		for _, e := range endPoints {
			if e.ApplicationName == appName {
				resultEndpointName = e.Name
			}
		}
		if resultEndpointName == "" {
			continue
		}

		// Now gather the goal state.
		for _, e := range endPoints {
			var key string
			app, err := u.st.Application(e.ApplicationName)
			if err == nil {
				key = app.Name()
			} else if errors.Is(err, errors.NotFound) {
				u.logger.Debugf(context.TODO(), "application %q must be a remote application.", e.ApplicationName)
				remoteApplication, err := u.st.RemoteApplication(e.ApplicationName)
				if err != nil {
					return nil, err
				}
				var ok bool
				key, ok = remoteApplication.URL()
				if !ok {
					// If we are on the offering side of a remote relation, don't show anything
					// in goal state for that relation.
					continue
				}
			} else {
				return nil, err
			}

			// We don't show units for the same application as we are currently processing.
			if key == appName {
				continue
			}

			goalState := params.GoalStateStatus{}
			goalState.Status = statusInfo.Status.String()
			goalState.Since = statusInfo.Since
			relationGoalState := result[e.Name]
			if relationGoalState == nil {
				relationGoalState = params.UnitsGoalState{}
			}
			relationGoalState[key] = goalState

			// For local applications, add in the status of units as well.
			if app != nil {
				units, err := u.goalStateUnits(ctx, app, principalName)
				if err != nil {
					return nil, err
				}
				for unitName, unitGS := range units {
					relationGoalState[unitName] = unitGS
				}
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
func (u *UniterAPI) goalStateUnits(ctx context.Context, app *state.Application, principalName string) (params.UnitsGoalState, error) {

	// TODO(units) - add service method for AllUnits
	allUnits, err := app.AllUnits()
	if err != nil {
		return nil, err
	}

	appID, err := u.applicationService.GetApplicationIDByName(ctx, app.Name())
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		return nil, errors.NotFoundf("application %q", app.Name())
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	unitWorkloadStatuses, err := u.applicationService.GetUnitWorkloadStatusesForApplication(ctx, appID)
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		return nil, errors.NotFoundf("application %q", app.Name())
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	unitsGoalState := params.UnitsGoalState{}
	for _, unit := range allUnits {
		// Ignore subordinates belonging to other units.
		pn, ok := unit.PrincipalName()
		if ok && pn != principalName {
			continue
		}
		unitLife := unit.Life()
		if unitLife == state.Dead {
			// only show Alive and Dying units
			u.logger.Debugf(ctx, "unit %q is dead, ignore it.", unit.Name())
			continue
		}
		unitGoalState := params.GoalStateStatus{}
		unitName, err := coreunit.NewName(unit.Name())
		if err != nil {
			return nil, errors.Trace(err)
		}
		statusInfo, ok := unitWorkloadStatuses[unitName]
		if !ok {
			return nil, errors.Errorf("status for unit %q not found", unitName)
		}
		unitGoalState.Status = statusInfo.Status.String()
		if unitLife == state.Dying {
			unitGoalState.Status = unitLife.String()
		}
		unitGoalState.Since = statusInfo.Since
		unitsGoalState[unit.Name()] = unitGoalState
	}

	return unitsGoalState, nil
}

// WatchConfigSettingsHash returns a StringsWatcher that yields a hash
// of the config values every time the config changes. The uniter can
// save this hash and use it to decide whether the config-changed hook
// needs to be run (or whether this was just an agent restart with no
// substantive config change).
func (u *UniterAPI) WatchConfigSettingsHash(ctx context.Context, args params.Entities) (params.StringsWatchResults, error) {
	getWatcher := func(unit *state.Unit) (state.StringsWatcher, error) {
		return unit.WatchConfigSettingsHash()
	}
	result, err := u.watchHashes(args, getWatcher)
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
	getWatcher := func(unit *state.Unit) (state.StringsWatcher, error) {
		return unit.WatchApplicationConfigSettingsHash()
	}
	result, err := u.watchHashes(args, getWatcher)
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
	getWatcher := func(unit *state.Unit) (state.StringsWatcher, error) {
		if !unit.ShouldBeAssigned() {
			app, err := unit.Application()
			if err != nil {
				return nil, err
			}
			return app.WatchServiceAddressesHash(), nil
		}
		return unit.WatchMachineAndEndpointAddressesHash()
	}
	result, err := u.watchHashes(args, getWatcher)
	if err != nil {
		return params.StringsWatchResults{}, errors.Trace(err)
	}
	return result, nil
}

func (u *UniterAPI) watchHashes(args params.Entities, getWatcher func(u *state.Unit) (state.StringsWatcher, error)) (params.StringsWatchResults, error) {
	result := params.StringsWatchResults{
		Results: make([]params.StringsWatchResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit()
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
			watcherId, changes, err = u.watchOneUnitHashes(tag, getWatcher)
		}
		result.Results[i].StringsWatcherId = watcherId
		result.Results[i].Changes = changes
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func (u *UniterAPI) watchOneUnitHashes(tag names.UnitTag, getWatcher func(u *state.Unit) (state.StringsWatcher, error)) (string, []string, error) {
	unit, err := u.getUnit(tag)
	if err != nil {
		return "", nil, err
	}
	w, err := getWatcher(unit)
	if err != nil {
		return "", nil, errors.Trace(err)
	}
	// Consume the initial event.
	if changes, ok := <-w.Changes(); ok {
		return u.resources.Register(w), changes, nil
	}
	return "", nil, statewatcher.EnsureErr(w)
}

// CloudAPIVersion returns the cloud API version, if available.
func (u *UniterAPI) CloudAPIVersion(ctx context.Context) (params.StringResult, error) {
	result := params.StringResult{}

	configGetter := stateenvirons.EnvironConfigGetter{
		Model:              u.environConfigGetterModel,
		NewContainerBroker: u.containerBrokerFunc,
		CloudService:       u.cloudService,
		CredentialService:  u.credentialService,
		ModelConfigService: u.modelConfigService,
	}
	spec, err := configGetter.CloudSpec(ctx)
	if err != nil {
		return result, apiservererrors.ServerError(err)
	}
	apiVersion, err := configGetter.CloudAPIVersion(spec)
	if err != nil {
		return result, apiservererrors.ServerError(err)
	}
	result.Result = apiVersion
	return result, err
}

// UpdateNetworkInfo refreshes the network settings for a unit's bound
// endpoints.
func (u *UniterAPI) UpdateNetworkInfo(ctx context.Context, args params.Entities) (params.ErrorResults, error) {
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}

	res := make([]params.ErrorResult, len(args.Entities))
	for i, entity := range args.Entities {
		unitTag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			res[i].Error = apiservererrors.ServerError(err)
			continue
		}

		if !canAccess(unitTag) {
			res[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}

		if err = u.updateUnitNetworkInfo(ctx, unitTag); err != nil {
			res[i].Error = apiservererrors.ServerError(err)
		}
	}

	return params.ErrorResults{Results: res}, nil
}

func (u *UniterAPI) updateUnitNetworkInfo(ctx context.Context, unitTag names.UnitTag) error {
	unit, err := u.getUnit(unitTag)
	if err != nil {
		return errors.Trace(err)
	}

	modelOp, err := u.updateUnitNetworkInfoOperation(ctx, unitTag, unit)
	if err != nil {
		return err
	}
	return u.st.ApplyOperation(modelOp)
}

func (u *UniterAPI) updateUnitNetworkInfoOperation(ctx context.Context, unitTag names.UnitTag, unit *state.Unit) (state.ModelOperation, error) {
	joinedRelations, err := unit.RelationsJoined()
	if err != nil {
		return nil, errors.Trace(err)
	}

	modelOps := make([]state.ModelOperation, len(joinedRelations))
	for idx, rel := range joinedRelations {
		relUnit, err := rel.Unit(unit)
		if err != nil {
			return nil, errors.Trace(err)
		}

		relSettings, err := relUnit.Settings()
		if err != nil {
			return nil, errors.Trace(err)
		}

		netInfo, err := NewNetworkInfo(ctx, u.st, u.clock, u.networkService, u.modelConfigService, unitTag, u.logger)
		if err != nil {
			return nil, err
		}

		_, ingressAddresses, egressSubnets, err := netInfo.NetworksForRelation(relUnit.Endpoint().Name, rel)
		if err != nil {
			return nil, errors.Trace(err)
		}

		if len(ingressAddresses) == 0 {
			relSettings.Delete("private-address")
			relSettings.Delete("ingress-address")
		} else {
			ingressAddress := ingressAddresses[0].Value
			relSettings.Set("private-address", ingressAddress)
			relSettings.Set("ingress-address", ingressAddress)
		}

		if len(egressSubnets) == 0 {
			relSettings.Delete("egress-subnets")
		} else {
			relSettings.Set("egress-subnets", strings.Join(egressSubnets, ","))
		}

		modelOps[idx] = relSettings.WriteOperation()
	}

	return state.ComposeModelOperations(modelOps...), nil
}

// CommitHookChanges batches together all required API calls for applying
// a set of changes after a hook successfully completes and executes them in a
// single transaction.
func (u *UniterAPI) CommitHookChanges(ctx context.Context, args params.CommitHookChangesArgs) (params.ErrorResults, error) {
	canAccessUnit, err := u.accessUnit()
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
				u.logger.Errorf(context.TODO(), "%s: %v", unitTag, err)
			}
			res[i].Error = apiservererrors.ServerError(err)
		}
	}

	return params.ErrorResults{Results: res}, nil
}

func (u *UniterAPI) commitHookChangesForOneUnit(ctx context.Context, unitTag names.UnitTag, changes params.CommitHookChangesArg, canAccessUnit, canAccessApp common.AuthFunc) error {
	unit, err := u.getUnit(unitTag)
	if err != nil {
		return internalerrors.Capture(err)
	}

	var modelOps []state.ModelOperation

	if changes.UpdateNetworkInfo {
		modelOp, err := u.updateUnitNetworkInfoOperation(ctx, unitTag, unit)
		if err != nil {
			return internalerrors.Capture(err)
		}
		modelOps = append(modelOps, modelOp)
	}

	for _, rus := range changes.RelationUnitSettings {
		// Ensure the unit in the unit settings matches the root unit name
		if rus.Unit != changes.Tag {
			return apiservererrors.ErrPerm
		}
		modelOp, err := u.updateUnitAndApplicationSettingsOp(rus, canAccessUnit)
		if err != nil {
			return internalerrors.Capture(err)
		}
		modelOps = append(modelOps, modelOp)
	}

	if len(changes.OpenPorts)+len(changes.ClosePorts) > 0 {
		modelInfo, err := u.modelInfoService.GetModelInfo(ctx)
		if err != nil {
			return internalerrors.Capture(err)
		}
		openPorts := network.GroupedPortRanges{}
		for _, r := range changes.OpenPorts {
			// Ensure the tag in the port open request matches the root unit name
			if r.Tag != changes.Tag {
				return apiservererrors.ErrPerm
			}

			// ICMP is not supported on CAAS models.
			if modelInfo.Type == model.CAAS && r.Protocol == "icmp" {
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

		curCons, err := unitStorageConstraints(u.StorageAPI.backend, unitTag)
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

// WatchInstanceData is a shim to call the LXDProfileAPIv2 version of this method.
func (u *UniterAPI) WatchInstanceData(ctx context.Context, args params.Entities) (params.NotifyWatchResults, error) {
	return u.lxdProfileAPI.WatchInstanceData(ctx, args)
}

// LXDProfileName is a shim to call the LXDProfileAPIv2 version of this method.
func (u *UniterAPI) LXDProfileName(ctx context.Context, args params.Entities) (params.StringResults, error) {
	return u.lxdProfileAPI.LXDProfileName(ctx, args)
}

// LXDProfileRequired is a shim to call the LXDProfileAPIv2 version of this method.
func (u *UniterAPI) LXDProfileRequired(ctx context.Context, args params.CharmURLs) (params.BoolResults, error) {
	return u.lxdProfileAPI.LXDProfileRequired(ctx, args)
}

// CanApplyLXDProfile is a shim to call the LXDProfileAPIv2 version of this method.
func (u *UniterAPI) CanApplyLXDProfile(ctx context.Context, args params.Entities) (params.BoolResults, error) {
	return u.lxdProfileAPI.CanApplyLXDProfile(ctx, args)
}

// APIHostPorts returns the API server addresses.
func (u *UniterAPI) APIHostPorts(ctx context.Context) (result params.APIHostPortsResult, err error) {
	controllerConfig, err := u.controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return result, errors.Trace(err)
	}

	return u.APIAddresser.APIHostPorts(ctx, controllerConfig)
}

// APIAddresses returns the list of addresses used to connect to the API.
func (u *UniterAPI) APIAddresses(ctx context.Context) (result params.StringsResult, err error) {
	controllerConfig, err := u.controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return result, errors.Trace(err)
	}

	return u.APIAddresser.APIAddresses(ctx, controllerConfig)
}

// WatchApplication starts an NotifyWatcher for an application.
// WatchApplication is not implemented in the UniterAPIv20 facade.
func (u *UniterAPI) WatchApplication(ctx context.Context, entity params.Entity) (params.NotifyWatchResult, error) {
	canWatch, err := u.accessApplication()
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
func (u *UniterAPI) WatchUnit(ctx context.Context, entity params.Entity) (params.NotifyWatchResult, error) {
	canWatch, err := u.accessUnit()
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

	watcher, err := u.watchUnit(tag)
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
	canWatch, err := u.accessUnitOrApplication()
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
		switch tag.(type) {
		case names.ApplicationTag:
			watcher, err = u.applicationService.WatchApplication(ctx, tag.Id())
		default:
			watcher, err = u.watchUnit(tag)
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
func (u *UniterAPI) watchUnit(tag names.Tag) (watcher.NotifyWatcher, error) {
	entity0, err := u.st.FindEntity(tag)
	if err != nil {
		return nil, err
	}
	entity, ok := entity0.(state.NotifyWatcherFactory)
	if !ok {
		return nil, apiservererrors.NotSupportedError(tag, "watching")
	}
	watcher := entity.Watch()
	return watcher, err
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
