// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/retry"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/cloudspec"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	leadershipapiserver "github.com/juju/juju/apiserver/facades/agent/leadership"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/life"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
	statewatcher "github.com/juju/juju/state/watcher"
)

// UniterAPI implements the latest version (v18) of the Uniter API.
type UniterAPI struct {
	*common.LifeGetter
	*StatusAPI
	*common.DeadEnsurer
	*common.AgentEntityWatcher
	*common.APIAddresser
	*common.MongoModelWatcher
	*common.RebootRequester
	*common.UpgradeSeriesAPI
	*common.UnitStateAPI
	*leadershipapiserver.LeadershipSettingsAccessor

	lxdProfileAPI           *LXDProfileAPIv2
	m                       *state.Model
	st                      *state.State
	cloudService            CloudService
	credentialService       CredentialService
	controllerConfigService ControllerConfigService
	modelConfigService      ModelConfigService
	modelInfoService        ModelInfoService
	secretService           SecretService
	networkService          NetworkService
	unitRemover             UnitRemover
	clock                   clock.Clock
	auth                    facade.Authorizer
	resources               facade.Resources
	leadershipChecker       leadership.Checker
	accessUnit              common.GetAuthFunc
	accessApplication       common.GetAuthFunc
	accessMachine           common.GetAuthFunc
	containerBrokerFunc     caas.NewContainerBrokerFunc
	*StorageAPI
	store objectstore.ObjectStore

	// A cloud spec can only be accessed for the model of the unit or
	// application that is authorised for this API facade.
	// We do not need to use an AuthFunc, because we do not need to pass a tag.
	accessCloudSpec func() (func() bool, error)
	cloudSpecer     cloudspec.CloudSpecer

	logger corelogger.Logger
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
		machPortRanges, err := u.getOneMachineOpenedPortRanges(canAccess, entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		result.Results[i].UnitPortRanges = make(map[string][]params.OpenUnitPortRangesByEndpoint)
		for unitName, unitPortRanges := range machPortRanges.ByUnit() {
			unitTag := names.NewUnitTag(unitName).String()
			for endpointName, portRanges := range unitPortRanges.ByEndpoint() {
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

func (u *UniterAPI) getOneMachineOpenedPortRanges(canAccess common.AuthFunc, machineTag string) (state.MachinePortRanges, error) {
	tag, err := names.ParseMachineTag(machineTag)
	if err != nil {
		return nil, apiservererrors.ErrPerm
	}
	if !canAccess(tag) {
		return nil, apiservererrors.ErrPerm
	}
	machine, err := u.getMachine(tag)
	if err != nil {
		return nil, err
	}
	return machine.OpenedPortRanges()
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

	unit, err := u.st.Unit(authTag.Id())
	if err != nil {
		result.Results[0].Error = apiservererrors.ServerError(err)
		return result, nil
	}
	openedPortRanges, err := unit.OpenedPortRanges()
	if err != nil {
		result.Results[0].Error = apiservererrors.ServerError(err)
		return result, nil
	}
	result.Results[0].UnitPortRanges = make(map[string][]params.OpenUnitPortRangesByEndpoint)
	unitTag := unit.Tag().String()
	for endpointName, portRanges := range openedPortRanges.ByEndpoint() {
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

func (u *UniterAPI) getMachine(tag names.MachineTag) (*state.Machine, error) {
	return u.st.Machine(tag.Id())
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

var getZone = func(st *state.State, tag names.Tag) (string, error) {
	unit, err := st.Unit(tag.Id())
	if err != nil {
		return "", errors.Trace(err)
	}
	zone, err := unit.AvailabilityZone()
	return zone, errors.Trace(err)
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
			zone, err = getZone(u.st, tag)
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
				unit    *state.Unit
				removed bool
			)
			unit, err = u.getUnit(tag)
			if err == nil {
				removed, err = unit.DestroyMaybeRemove(u.store)
				if err == nil && removed {
					err = u.unitRemover.DeleteUnit(ctx, unit.Name())
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
		ver, err := u.charmModifiedVersion(entity.Tag, canAccess)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results.Results[i].Result = ver
	}
	return results, nil
}

func (u *UniterAPI) charmModifiedVersion(tagStr string, canAccess func(names.Tag) bool) (int, error) {
	tag, err := names.ParseTag(tagStr)
	if err != nil {
		return -1, apiservererrors.ErrPerm
	}
	if !canAccess(tag) {
		return -1, apiservererrors.ErrPerm
	}
	unitOrApplication, err := u.st.FindEntity(tag)
	if err != nil {
		return -1, err
	}
	var application *state.Application
	switch entity := unitOrApplication.(type) {
	case *state.Application:
		application = entity
	case *state.Unit:
		application, err = entity.Application()
		if err != nil {
			return -1, err
		}
	default:
		return -1, errors.BadRequestf("type %T does not have a CharmModifiedVersion", entity)
	}
	return application.CharmModifiedVersion(), nil
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
				// TODO(cache) - we'd wait for the model cache to receive the change.
				// But we're not using the model cache at the moment.
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
			// TODO(cache) - we were using the model cache but due to
			// issues with propagating the charm URL, use the state model.
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
	// The charm in state may only be a placeholder when this call is made.
	// Ideally, the unit agent would not be started until the charm is fully available,
	// but that's not currently the case and it doesn't hurt to be defensive here regardless.
	// We'll retry the sha256 lookup if the charm is still pending and therefore not found.
	var sha string
	err := retry.Call(retry.CallArgs{
		Func: func() error {
			sch, err := u.st.Charm(curl)
			if err != nil {
				return errors.Trace(err)
			}
			sha = sch.BundleSha256()
			if sha == "" {
				return errors.NotFoundf("downloaded charm %q", curl)
			}
			return nil
		},
		IsFatalError: func(err error) bool {
			return !errors.Is(err, errors.NotFound)
		},
		Stop:     ctx.Done(),
		Delay:    3 * time.Second,
		Attempts: 20,
		Clock:    u.clock,
	})
	if errors.Is(err, errors.NotFound) {
		return "", apiservererrors.ErrPerm
	}
	return sha, errors.Trace(err)
}

// Relation returns information about all given relation/unit pairs,
// including their id, key and the local endpoint.
func (u *UniterAPI) Relation(ctx context.Context, args params.RelationUnits) (params.RelationResults, error) {
	result := params.RelationResults{
		Results: make([]params.RelationResult, len(args.RelationUnits)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.RelationResults{}, err
	}
	for i, rel := range args.RelationUnits {
		relParams, err := u.getOneRelation(canAccess, rel.Relation, rel.Unit)
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
// endpoint.
func (u *UniterAPI) RelationById(ctx context.Context, args params.RelationIds) (params.RelationResults, error) {
	result := params.RelationResults{
		Results: make([]params.RelationResult, len(args.RelationIds)),
	}
	for i, relId := range args.RelationIds {
		relParams, err := u.getOneRelationById(relId)
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

	oneRelationUnitStatus := func(rel *state.Relation, unit *state.Unit) (params.RelationUnitStatus, error) {
		rus := params.RelationUnitStatus{
			RelationTag: rel.Tag().String(),
			Suspended:   rel.Suspended(),
		}
		ru, err := rel.Unit(unit)
		if err != nil {
			return params.RelationUnitStatus{}, errors.Trace(err)
		}
		inScope, err := ru.InScope()
		if err != nil {
			return params.RelationUnitStatus{}, errors.Trace(err)
		}
		rus.InScope = inScope
		return rus, nil
	}

	relationResults := func(unit *state.Unit) ([]params.RelationUnitStatus, error) {
		var ruStatus []params.RelationUnitStatus
		app, err := unit.Application()
		if err != nil {
			return nil, errors.Trace(err)
		}
		relations, err := app.Relations()
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, rel := range relations {
			rus, err := oneRelationUnitStatus(rel, unit)
			if err != nil {
				return nil, errors.Trace(err)
			}
			ruStatus = append(ruStatus, rus)
		}
		return ruStatus, nil
	}

	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		err = apiservererrors.ErrPerm
		if canRead(tag) {
			var unit *state.Unit
			unit, err = u.getUnit(tag)
			if err == nil {
				result.Results[i].RelationResults, err = relationResults(unit)
			}
		}
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
			u.logger.Debugf("ignoring %q EnterScope for %q - unit has invalid principal %q",
				unit.Name(), rel.String(), principalName)
			return nil
		}

		netInfo, err := NewNetworkInfo(ctx, u.st, u.networkService, u.modelConfigService, unitTag, u.logger)
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
			u.logger.Warningf("cannot set ingress/egress addresses for unit %v in relation %v: %v",
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

	readOneSettings := func(arg params.RelationUnit) (params.Settings, error) {
		tag, err := names.ParseTag(arg.Unit)
		if err != nil {
			return nil, apiservererrors.ErrPerm
		}

		var settings map[string]interface{}

		switch tag := tag.(type) {
		case names.UnitTag:
			var relUnit *state.RelationUnit
			relUnit, err = u.getRelationUnit(canAccessUnit, arg.Relation, tag)
			if err != nil {
				return nil, errors.Trace(err)
			}
			var node *state.Settings
			node, err = relUnit.Settings()
			if err != nil {
				return nil, errors.Trace(err)
			}
			settings = node.Map()
		case names.ApplicationTag:
			// Emulate a ReadLocalApplicationSettings call where
			// the currently authenticated tag is implicitly
			// assumed to be the requesting unit.
			authTag := u.auth.GetAuthTag()
			if authTag.Kind() != names.UnitTagKind {
				// See LP1876097
				return nil, apiservererrors.ErrPerm
			}
			settings, err = u.readLocalApplicationSettings(arg.Relation, tag, authTag.(names.UnitTag))
		default:
			return nil, apiservererrors.ErrPerm
		}

		if err != nil {
			return nil, errors.Trace(err)
		}
		return convertRelationSettings(settings)
	}

	for i, arg := range args.RelationUnits {
		settings, err := readOneSettings(arg)
		result.Results[i].Error = apiservererrors.ServerError(err)
		result.Results[i].Settings = settings
	}
	return result, nil
}

// ReadLocalApplicationSettings returns the local application settings for a
// particular relation when invoked by the leader unit.
func (u *UniterAPI) ReadLocalApplicationSettings(ctx context.Context, arg params.RelationUnit) (params.SettingsResult, error) {
	var res params.SettingsResult

	unitTag, err := names.ParseUnitTag(arg.Unit)
	if err != nil {
		return res, errors.NotValidf("unit tag %q", arg.Unit)
	}

	inferredAppName, err := names.UnitApplication(unitTag.Id())
	if err != nil {
		return res, errors.NotValidf("inferred application name from %q", arg.Unit)
	}
	inferredAppTag := names.NewApplicationTag(inferredAppName)

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

	settings, err := u.readLocalApplicationSettings(arg.Relation, inferredAppTag, unitTag)
	if err != nil {
		return res, errors.Trace(err)
	}

	res.Settings, err = convertRelationSettings(settings)
	return res, errors.Trace(err)
}

// readLocalApplicationSettings attempts to access the local application data
// bag for the specified relation on appTag on behalf of unitTag. If the
// provided unitTag is not the leader, this method will return ErrPerm.
func (u *UniterAPI) readLocalApplicationSettings(relTag string, appTag names.ApplicationTag, unitTag names.UnitTag) (map[string]interface{}, error) {
	canAccessApp, err := u.accessApplication()
	if err != nil {
		return nil, errors.Trace(err)
	}

	relation, err := u.getRelation(relTag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	endpoints := relation.Endpoints()
	token := u.leadershipChecker.LeadershipCheck(appTag.Id(), unitTag.Id())

	canAccessSettings := func(appTag names.Tag) bool {
		if !canAccessApp(appTag) {
			return false
		}

		isPeerRelation := len(endpoints) == 1 && endpoints[0].Role == charm.RolePeer
		if isPeerRelation {
			return true
		}
		// For provider-requirer relations only allow the
		// leader unit to read the application settings.
		return token.Check() == nil
	}

	return u.getRelationAppSettings(canAccessSettings, relTag, appTag)
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

	readOneSettings := func(arg params.RelationUnitPair) (params.Settings, error) {
		unit, err := names.ParseUnitTag(arg.LocalUnit)
		if err != nil {
			return nil, apiservererrors.ErrPerm
		}
		relUnit, err := u.getRelationUnit(canAccess, arg.Relation, unit)
		if err != nil {
			return nil, errors.Trace(err)
		}
		remoteTag, err := names.ParseTag(arg.RemoteUnit)
		if err != nil {
			return nil, apiservererrors.ErrPerm
		}

		var settings map[string]interface{}

		switch tag := remoteTag.(type) {
		case names.UnitTag:
			var remoteUnit string
			remoteUnit, err = u.checkRemoteUnit(relUnit, tag)
			if err != nil {
				return nil, errors.Trace(err)
			}
			settings, err = relUnit.ReadSettings(remoteUnit)
		case names.ApplicationTag:
			settings, err = u.getRemoteRelationAppSettings(relUnit.Relation(), tag)
		default:
			return nil, apiservererrors.ErrPerm
		}
		if err != nil {
			return nil, errors.Trace(err)
		}
		return convertRelationSettings(settings)
	}

	for i, arg := range args.RelationUnitPairs {
		settings, err := readOneSettings(arg)
		result.Results[i].Error = apiservererrors.ServerError(err)
		result.Results[i].Settings = settings
	}

	return result, nil
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

	unitCache := make(map[string]*state.Unit)
	getUnit := func(tag string) (*state.Unit, error) {
		if unit, ok := unitCache[tag]; ok {
			return unit, nil
		}
		unitTag, err := names.ParseUnitTag(tag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		unit, err := u.st.Unit(unitTag.Id())
		if errors.Is(err, errors.NotFound) {
			return nil, apiservererrors.ErrPerm
		}
		if err != nil {
			return nil, errors.Trace(err)
		}
		unitCache[tag] = unit
		return unit, nil
	}

	checker := u.leadershipChecker
	changeOne := func(arg params.RelationStatusArg) error {
		// TODO(wallyworld) - the token should be passed to SetStatus() but the
		// interface method doesn't allow for that yet.
		unitTag := arg.UnitTag
		if unitTag == "" {
			// Older clients don't pass in the unit tag explicitly.
			unitTag = u.auth.GetAuthTag().String()
		}
		unit, err := getUnit(unitTag)
		if err != nil {
			return err
		}
		token := checker.LeadershipCheck(unit.ApplicationName(), unit.Name())
		if err := token.Check(); err != nil {
			return errors.Trace(err)
		}

		rel, err := u.st.Relation(arg.RelationId)
		if errors.Is(err, errors.NotFound) {
			return apiservererrors.ErrPerm
		} else if err != nil {
			return errors.Trace(err)
		}
		_, err = rel.Unit(unit)
		if errors.Is(err, errors.NotFound) {
			return apiservererrors.ErrPerm
		} else if err != nil {
			return errors.Trace(err)
		}
		// If we are transitioning from "suspending" to "suspended",
		// we retain any existing message so that if the user has
		// previously specified a reason for suspending, it is retained.
		message := arg.Message
		if message == "" && arg.Status == params.Suspended {
			current, err := rel.Status()
			if err != nil {
				return errors.Trace(err)
			}
			if current.Status == status.Suspending {
				message = current.Message
			}
		}
		return rel.SetStatus(status.StatusInfo{
			Status:  status.Status(arg.Status),
			Message: message,
		})
	}
	results := make([]params.ErrorResult, len(args.Args))
	for i, arg := range args.Args {
		err := changeOne(arg)
		results[i].Error = apiservererrors.ServerError(err)
	}
	statusResults.Results = results
	return statusResults, nil
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

func (u *UniterAPI) getOneRelationById(relId int) (params.RelationResult, error) {
	nothing := params.RelationResult{}
	rel, err := u.st.Relation(relId)
	if errors.Is(err, errors.NotFound) {
		return nothing, apiservererrors.ErrPerm
	} else if err != nil {
		return nothing, err
	}
	var applicationName string
	tag := u.auth.GetAuthTag()
	switch tag.(type) {
	case names.UnitTag:
		unit, err := u.st.Unit(tag.Id())
		if err != nil {
			return nothing, err
		}
		applicationName = unit.ApplicationName()
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

func (u *UniterAPI) prepareRelationResult(rel *state.Relation, applicationName string) (params.RelationResult, error) {
	nothing := params.RelationResult{}
	ep, err := rel.Endpoint(applicationName)
	if err != nil {
		// An error here means the unit's application is not part of the
		// relation.
		return nothing, err
	}
	var otherAppName string
	otherEndpoints, err := rel.RelatedEndpoints(applicationName)
	if err != nil {
		return nothing, err
	}
	for _, otherEp := range otherEndpoints {
		otherAppName = otherEp.ApplicationName
	}
	return params.RelationResult{
		Id:        rel.Id(),
		Key:       rel.String(),
		Life:      life.Value(rel.Life().String()),
		Suspended: rel.Suspended(),
		Endpoint: params.Endpoint{
			ApplicationName: ep.ApplicationName,
			Relation:        params.NewCharmRelation(ep.Relation),
		},
		OtherApplication: otherAppName,
	}, nil
}

func (u *UniterAPI) getOneRelation(canAccess common.AuthFunc, relTag, unitTag string) (params.RelationResult, error) {
	nothing := params.RelationResult{}
	tag, err := names.ParseUnitTag(unitTag)
	if err != nil {
		return nothing, apiservererrors.ErrPerm
	}
	rel, unit, err := u.getRelationAndUnit(canAccess, relTag, tag)
	if err != nil {
		return nothing, err
	}
	return u.prepareRelationResult(rel, unit.ApplicationName())
}

func (u *UniterAPI) getRelationAppSettings(canAccess common.AuthFunc, relTag string, appTag names.ApplicationTag) (map[string]interface{}, error) {
	tag, err := names.ParseRelationTag(relTag)
	if err != nil {
		return nil, apiservererrors.ErrPerm
	}
	rel, err := u.st.KeyRelation(tag.Id())
	if errors.Is(err, errors.NotFound) {
		return nil, apiservererrors.ErrPerm
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	if !canAccess(appTag) {
		return nil, apiservererrors.ErrPerm
	}

	settings, err := rel.ApplicationSettings(appTag.Id())
	if errors.Is(err, errors.NotFound) {
		return nil, apiservererrors.ErrPerm
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	return settings, nil
}

func (u *UniterAPI) getRemoteRelationAppSettings(rel *state.Relation, appTag names.ApplicationTag) (map[string]interface{}, error) {
	// Check that the application is actually remote.
	var localAppName string
	switch tag := u.auth.GetAuthTag().(type) {
	case names.UnitTag:
		unit, err := u.st.Unit(tag.Id())
		if err != nil {
			return nil, errors.Trace(err)
		}
		localAppName = unit.ApplicationName()
	case names.ApplicationTag:
		localAppName = tag.Id()
	}
	relatedEPs, err := rel.RelatedEndpoints(localAppName)
	if err != nil {
		return nil, apiservererrors.ErrPerm
	}

	var isRelatedToLocalApp bool
	for _, ep := range relatedEPs {
		if appTag.Id() == ep.ApplicationName {
			isRelatedToLocalApp = true
			break
		}
	}

	if !isRelatedToLocalApp {
		return nil, apiservererrors.ErrPerm
	}

	return rel.ApplicationSettings(appTag.Id())
}

func (u *UniterAPI) destroySubordinates(ctx context.Context, principal *state.Unit) error {
	subordinates := principal.SubordinateNames()
	for _, subName := range subordinates {
		unit, err := u.getUnit(names.NewUnitTag(subName))
		if err != nil {
			return err
		}
		removed, err := unit.DestroyMaybeRemove(u.store)
		if err != nil {
			return err
		}
		if removed {
			if err := u.unitRemover.DeleteUnit(ctx, subName); err != nil {
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

func (u *UniterAPI) checkRemoteUnit(relUnit *state.RelationUnit, remoteUnitTag names.UnitTag) (string, error) {
	// Make sure the unit is indeed remote.
	switch tag := u.auth.GetAuthTag().(type) {
	case names.UnitTag:
		if remoteUnitTag == tag {
			return "", apiservererrors.ErrPerm
		}
	case names.ApplicationTag:
		endpoints := relUnit.Relation().Endpoints()
		isPeerRelation := len(endpoints) == 1 && endpoints[0].Role == charm.RolePeer
		if isPeerRelation {
			break
		}
		// If called by an application agent, we need
		// to check the units of the application.
		app, err := u.st.Application(tag.Name)
		if err != nil {
			return "", errors.Trace(err)
		}
		allUnits, err := app.AllUnits()
		if err != nil {
			return "", errors.Trace(err)
		}
		for _, unit := range allUnits {
			if remoteUnitTag == unit.Tag() {
				return "", apiservererrors.ErrPerm
			}
		}
	}

	// Check remoteUnit is indeed related. Note that we don't want to actually get
	// the *Unit, because it might have been removed; but its relation settings will
	// persist until the relation itself has been removed (and must remain accessible
	// because the local unit's view of reality may be time-shifted).
	remoteUnitName := remoteUnitTag.Id()
	remoteApplicationName, err := names.UnitApplication(remoteUnitName)
	if err != nil {
		return "", apiservererrors.ErrPerm
	}
	rel := relUnit.Relation()
	_, err = rel.RelatedEndpoints(remoteApplicationName)
	if err != nil {
		return "", apiservererrors.ErrPerm
	}
	return remoteUnitName, nil
}

func convertRelationSettings(settings map[string]interface{}) (params.Settings, error) {
	result := make(params.Settings)
	for k, v := range settings {
		// All relation settings should be strings.
		sval, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("unexpected relation setting %q: expected string, got %T", k, v)
		}
		result[k] = sval
	}
	return result, nil
}

func leadershipSettingsAccessorFactory(
	st *state.State,
	checker leadership.Checker,
	resources facade.Resources,
	auth facade.Authorizer,
) *leadershipapiserver.LeadershipSettingsAccessor {
	registerWatcher := func(applicationId string) (string, error) {
		application, err := st.Application(applicationId)
		if err != nil {
			return "", err
		}
		w := application.WatchLeaderSettings()
		if _, ok := <-w.Changes(); ok {
			return resources.Register(w), nil
		}
		return "", statewatcher.EnsureErr(w)
	}
	getSettings := func(applicationId string) (map[string]string, error) {
		application, err := st.Application(applicationId)
		if err != nil {
			return nil, err
		}
		return application.LeaderSettings()
	}
	writeSettings := func(token leadership.Token, applicationId string, settings map[string]string) error {
		application, err := st.Application(applicationId)
		if err != nil {
			return err
		}
		return application.UpdateLeaderSettings(token, settings)
	}
	return leadershipapiserver.NewLeadershipSettingsAccessor(
		auth,
		registerWatcher,
		getSettings,
		checker.LeadershipCheck,
		writeSettings,
	)
}

// V4 specific methods.

//  specific methods - the new NetworkInfo and
// WatchUnitRelations methods.

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

	netInfo, err := NewNetworkInfo(ctx, u.st, u.networkService, u.modelConfigService, unitTag, u.logger)
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
		result.Results[i].Result, err = u.oneGoalState(unit)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
		}
	}
	return result, nil
}

// oneGoalState creates the goal state for a given unit.
func (u *UniterAPI) oneGoalState(unit *state.Unit) (*params.GoalState, error) {
	app, err := unit.Application()
	if err != nil {
		return nil, err
	}

	gs := params.GoalState{}
	gs.Units, err = u.goalStateUnits(app, unit.Name())
	if err != nil {
		return nil, err
	}
	allRelations, err := app.Relations()
	if err != nil {
		return nil, err
	}
	if allRelations != nil {
		gs.Relations, err = u.goalStateRelations(app.Name(), unit.Name(), allRelations)
		if err != nil {
			return nil, err
		}
	}
	return &gs, nil
}

// goalStateRelations creates the structure with all the relations between endpoints in an application.
func (u *UniterAPI) goalStateRelations(appName, principalName string, allRelations []*state.Relation) (map[string]params.UnitsGoalState, error) {

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
				u.logger.Debugf("application %q must be a remote application.", e.ApplicationName)
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
				units, err := u.goalStateUnits(app, principalName)
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
func (u *UniterAPI) goalStateUnits(app *state.Application, principalName string) (params.UnitsGoalState, error) {

	allUnits, err := app.AllUnits()
	if err != nil {
		return nil, err
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
			u.logger.Debugf("unit %q is dead, ignore it.", unit.Name())
			continue
		}
		unitGoalState := params.GoalStateStatus{}
		statusInfo, err := unit.Status()
		if err != nil {
			return nil, errors.Trace(err)
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
		Model: u.m, NewContainerBroker: u.containerBrokerFunc, CloudService: u.cloudService, CredentialService: u.credentialService}
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

		netInfo, err := NewNetworkInfo(ctx, u.st, u.networkService, u.modelConfigService, unitTag, u.logger)
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
				u.logger.Errorf("%s: %v", unitTag, err)
			}
			res[i].Error = apiservererrors.ServerError(err)
		}
	}

	return params.ErrorResults{Results: res}, nil
}

func (u *UniterAPI) commitHookChangesForOneUnit(ctx context.Context, unitTag names.UnitTag, changes params.CommitHookChangesArg, canAccessUnit, canAccessApp common.AuthFunc) error {
	unit, err := u.getUnit(unitTag)
	if err != nil {
		return errors.Trace(err)
	}

	ctrlCfg, err := u.controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	var modelOps []state.ModelOperation

	if changes.UpdateNetworkInfo {
		modelOp, err := u.updateUnitNetworkInfoOperation(ctx, unitTag, unit)
		if err != nil {
			return errors.Trace(err)
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
			return errors.Trace(err)
		}
		modelOps = append(modelOps, modelOp)
	}

	if len(changes.OpenPorts)+len(changes.ClosePorts) > 0 {
		pcp, err := unit.OpenedPortRanges()
		if err != nil {
			return errors.Trace(err)
		}

		for _, r := range changes.OpenPorts {
			// Ensure the tag in the port open request matches the root unit name
			if r.Tag != changes.Tag {
				return apiservererrors.ErrPerm
			}

			// Pre-2.9 clients (using V15 or V16 of this API) do
			// not populate the new Endpoint field; this
			// effectively opens the port for all endpoints and
			// emulates pre-2.9 behavior.
			pcp.Open(r.Endpoint, network.PortRange{
				FromPort: r.FromPort,
				ToPort:   r.ToPort,
				Protocol: r.Protocol,
			})
		}
		for _, r := range changes.ClosePorts {
			// Ensure the tag in the port close request matches the root unit name
			if r.Tag != changes.Tag {
				return apiservererrors.ErrPerm
			}

			// Pre-2.9 clients (using V15 or V16 of this API) do
			// not populate the new Endpoint field; this
			// effectively closes the port for all endpoints and
			// emulates pre-2.9 behavior.
			pcp.Close(r.Endpoint, network.PortRange{
				FromPort: r.FromPort,
				ToPort:   r.ToPort,
				Protocol: r.Protocol,
			})
		}
		modelOps = append(modelOps, pcp.Changes())
	}

	if changes.SetUnitState != nil {
		// Ensure the tag in the set state request matches the root unit name
		if changes.SetUnitState.Tag != changes.Tag {
			return apiservererrors.ErrPerm
		}

		newUS := state.NewUnitState()
		if changes.SetUnitState.CharmState != nil {
			newUS.SetCharmState(*changes.SetUnitState.CharmState)
		}

		// NOTE(achilleasa): The following state fields are not
		// presently populated by the uniter calls to this API as they
		// get persisted after the hook changes get committed. However,
		// they are still checked here for future use and for ensuring
		// symmetry with the SetState call (see apiserver/common).
		if changes.SetUnitState.UniterState != nil {
			newUS.SetUniterState(*changes.SetUnitState.UniterState)
		}
		if changes.SetUnitState.RelationState != nil {
			newUS.SetRelationState(*changes.SetUnitState.RelationState)
		}
		if changes.SetUnitState.StorageState != nil {
			newUS.SetStorageState(*changes.SetUnitState.StorageState)
		}
		if changes.SetUnitState.SecretState != nil {
			newUS.SetSecretState(*changes.SetUnitState.SecretState)
		}

		modelOp := unit.SetStateOperation(
			newUS,
			state.UnitStateSizeLimits{
				MaxCharmStateSize: ctrlCfg.MaxCharmStateSize(),
				MaxAgentStateSize: ctrlCfg.MaxAgentStateSize(),
			},
		)
		modelOps = append(modelOps, modelOp)
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
	return u.lxdProfileAPI.WatchInstanceData(args)
}

// LXDProfileName is a shim to call the LXDProfileAPIv2 version of this method.
func (u *UniterAPI) LXDProfileName(ctx context.Context, args params.Entities) (params.StringResults, error) {
	return u.lxdProfileAPI.LXDProfileName(args)
}

// LXDProfileRequired is a shim to call the LXDProfileAPIv2 version of this method.
func (u *UniterAPI) LXDProfileRequired(ctx context.Context, args params.CharmURLs) (params.BoolResults, error) {
	return u.lxdProfileAPI.LXDProfileRequired(args)
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
