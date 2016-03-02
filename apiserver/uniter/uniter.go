// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package uniter implements the API interface used by the uniter worker.

package uniter

import (
	"fmt"
	"net/url"
	"path"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/apiserver/common"
	leadershipapiserver "github.com/juju/juju/apiserver/leadership"
	"github.com/juju/juju/apiserver/meterstatus"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/state/watcher"
)

var logger = loggo.GetLogger("juju.apiserver.uniter")

func init() {
	common.RegisterStandardFacade("Uniter", 3, NewUniterAPIV3)
}

// UniterAPIV3 implements the API version 3, used by the uniter worker.
type UniterAPIV3 struct {
	*common.LifeGetter
	*StatusAPI
	*common.DeadEnsurer
	*common.AgentEntityWatcher
	*common.APIAddresser
	*common.ModelWatcher
	*common.RebootRequester
	*leadershipapiserver.LeadershipSettingsAccessor
	meterstatus.MeterStatus

	st            *state.State
	auth          common.Authorizer
	resources     *common.Resources
	accessUnit    common.GetAuthFunc
	accessService common.GetAuthFunc
	unit          *state.Unit
	accessMachine common.GetAuthFunc
	StorageAPI
}

// NewUniterAPIV3 creates a new instance of the Uniter API, version 3.
func NewUniterAPIV3(st *state.State, resources *common.Resources, authorizer common.Authorizer) (*UniterAPIV3, error) {
	if !authorizer.AuthUnitAgent() {
		return nil, common.ErrPerm
	}
	var unit *state.Unit
	var err error
	switch tag := authorizer.GetAuthTag().(type) {
	case names.UnitTag:
		unit, err = st.Unit(tag.Id())
		if err != nil {
			return nil, errors.Trace(err)
		}
	default:
		return nil, errors.Errorf("expected names.UnitTag, got %T", tag)
	}
	accessUnit := func() (common.AuthFunc, error) {
		return authorizer.AuthOwner, nil
	}
	accessService := func() (common.AuthFunc, error) {
		switch tag := authorizer.GetAuthTag().(type) {
		case names.UnitTag:
			entity, err := st.Unit(tag.Id())
			if err != nil {
				return nil, errors.Trace(err)
			}
			serviceName := entity.ServiceName()
			serviceTag := names.NewServiceTag(serviceName)
			return func(tag names.Tag) bool {
				return tag == serviceTag
			}, nil
		default:
			return nil, errors.Errorf("expected names.UnitTag, got %T", tag)
		}
	}
	accessMachine := func() (common.AuthFunc, error) {
		switch tag := authorizer.GetAuthTag().(type) {
		case names.UnitTag:
			entity, err := st.Unit(tag.Id())
			if err != nil {
				return nil, errors.Trace(err)
			}
			machineId, err := entity.AssignedMachineId()
			if err != nil {
				return nil, errors.Trace(err)
			}
			machineTag := names.NewMachineTag(machineId)
			return func(tag names.Tag) bool {
				return tag == machineTag
			}, nil
		default:
			return nil, errors.Errorf("expected names.UnitTag, got %T", tag)
		}
	}
	storageAPI, err := newStorageAPI(getStorageState(st), resources, accessUnit)
	if err != nil {
		return nil, err
	}
	msAPI, err := meterstatus.NewMeterStatusAPI(st, resources, authorizer)
	if err != nil {
		return nil, errors.Annotate(err, "could not create meter status API handler")
	}
	accessUnitOrService := common.AuthEither(accessUnit, accessService)
	return &UniterAPIV3{
		LifeGetter:                 common.NewLifeGetter(st, accessUnitOrService),
		DeadEnsurer:                common.NewDeadEnsurer(st, accessUnit),
		AgentEntityWatcher:         common.NewAgentEntityWatcher(st, resources, accessUnitOrService),
		APIAddresser:               common.NewAPIAddresser(st, resources),
		ModelWatcher:               common.NewModelWatcher(st, resources, authorizer),
		RebootRequester:            common.NewRebootRequester(st, accessMachine),
		LeadershipSettingsAccessor: leadershipSettingsAccessorFactory(st, resources, authorizer),
		MeterStatus:                msAPI,
		// TODO(fwereade): so *every* unit should be allowed to get/set its
		// own status *and* its service's? This is not a pleasing arrangement.
		StatusAPI: NewStatusAPI(st, accessUnitOrService),

		st:            st,
		auth:          authorizer,
		resources:     resources,
		accessUnit:    accessUnit,
		accessService: accessService,
		accessMachine: accessMachine,
		unit:          unit,
		StorageAPI:    *storageAPI,
	}, nil
}

// AllMachinePorts returns all opened port ranges for each given
// machine (on all networks).
func (u *UniterAPIV3) AllMachinePorts(args params.Entities) (params.MachinePortsResults, error) {
	result := params.MachinePortsResults{
		Results: make([]params.MachinePortsResult, len(args.Entities)),
	}
	canAccess, err := u.accessMachine()
	if err != nil {
		return params.MachinePortsResults{}, err
	}
	for i, entity := range args.Entities {
		result.Results[i] = u.getOneMachinePorts(canAccess, entity.Tag)
	}
	return result, nil
}

// ServiceOwner returns the owner user for each given service tag.
func (u *UniterAPIV3) ServiceOwner(args params.Entities) (params.StringResults, error) {
	result := params.StringResults{
		Results: make([]params.StringResult, len(args.Entities)),
	}
	canAccess, err := u.accessService()
	if err != nil {
		return params.StringResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseServiceTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		if !canAccess(tag) {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		service, err := u.getService(tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		result.Results[i].Result = service.GetOwnerTag()
	}
	return result, nil
}

// AssignedMachine returns the machine tag for each given unit tag, or
// an error satisfying params.IsCodeNotAssigned when a unit has no
// assigned machine.
func (u *UniterAPIV3) AssignedMachine(args params.Entities) (params.StringResults, error) {
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
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		if !canAccess(tag) {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		unit, err := u.getUnit(tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		machineId, err := unit.AssignedMachineId()
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
		} else {
			result.Results[i].Result = names.NewMachineTag(machineId).String()
		}
	}
	return result, nil
}

func (u *UniterAPIV3) getMachine(tag names.MachineTag) (*state.Machine, error) {
	return u.st.Machine(tag.Id())
}

func (u *UniterAPIV3) getOneMachinePorts(canAccess common.AuthFunc, machineTag string) params.MachinePortsResult {
	tag, err := names.ParseMachineTag(machineTag)
	if err != nil {
		return params.MachinePortsResult{Error: common.ServerError(common.ErrPerm)}
	}
	if !canAccess(tag) {
		return params.MachinePortsResult{Error: common.ServerError(common.ErrPerm)}
	}
	machine, err := u.getMachine(tag)
	if err != nil {
		return params.MachinePortsResult{Error: common.ServerError(err)}
	}
	allPorts, err := machine.AllPorts()
	if err != nil {
		return params.MachinePortsResult{Error: common.ServerError(err)}
	}
	var resultPorts []params.MachinePortRange
	for _, ports := range allPorts {
		// AllPortRanges gives a map, but apis require a stable order
		// for results, so sort the port ranges.
		portRangesToUnits := ports.AllPortRanges()
		portRanges := make([]network.PortRange, 0, len(portRangesToUnits))
		for portRange := range portRangesToUnits {
			portRanges = append(portRanges, portRange)
		}
		network.SortPortRanges(portRanges)
		for _, portRange := range portRanges {
			unitName := portRangesToUnits[portRange]
			resultPorts = append(resultPorts, params.MachinePortRange{
				UnitTag:   names.NewUnitTag(unitName).String(),
				PortRange: params.FromNetworkPortRange(portRange),
			})
		}
	}
	return params.MachinePortsResult{
		Ports: resultPorts,
	}
}

// PublicAddress returns the public address for each given unit, if set.
func (u *UniterAPIV3) PublicAddress(args params.Entities) (params.StringResults, error) {
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
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
		if canAccess(tag) {
			var unit *state.Unit
			unit, err = u.getUnit(tag)
			if err == nil {
				var address network.Address
				address, err = unit.PublicAddress()
				if err == nil {
					result.Results[i].Result = address.Value
				} else if network.IsNoAddress(err) {
					err = common.NoAddressSetError(tag, "public")
				}
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// PrivateAddress returns the private address for each given unit, if set.
func (u *UniterAPIV3) PrivateAddress(args params.Entities) (params.StringResults, error) {
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
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
		if canAccess(tag) {
			var unit *state.Unit
			unit, err = u.getUnit(tag)
			if err == nil {
				var address network.Address
				address, err = unit.PrivateAddress()
				if err == nil {
					result.Results[i].Result = address.Value
				} else if network.IsNoAddress(err) {
					err = common.NoAddressSetError(tag, "private")
				}
			}
		}
		result.Results[i].Error = common.ServerError(err)
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
func (u *UniterAPIV3) AvailabilityZone(args params.Entities) (params.StringResults, error) {
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
			results.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
		if canAccess(tag) {
			var zone string
			zone, err = getZone(u.st, tag)
			if err == nil {
				results.Results[i].Result = zone
			}
		}
		results.Results[i].Error = common.ServerError(err)
	}

	return results, nil
}

// Resolved returns the current resolved setting for each given unit.
func (u *UniterAPIV3) Resolved(args params.Entities) (params.ResolvedModeResults, error) {
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
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
		if canAccess(tag) {
			var unit *state.Unit
			unit, err = u.getUnit(tag)
			if err == nil {
				result.Results[i].Mode = params.ResolvedMode(unit.Resolved())
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// ClearResolved removes any resolved setting from each given unit.
func (u *UniterAPIV3) ClearResolved(args params.Entities) (params.ErrorResults, error) {
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
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
		if canAccess(tag) {
			var unit *state.Unit
			unit, err = u.getUnit(tag)
			if err == nil {
				err = unit.ClearResolved()
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// GetPrincipal returns the result of calling PrincipalName() and
// converting it to a tag, on each given unit.
func (u *UniterAPIV3) GetPrincipal(args params.Entities) (params.StringBoolResults, error) {
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
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
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
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// Destroy advances all given Alive units' lifecycles as far as
// possible. See state/Unit.Destroy().
func (u *UniterAPIV3) Destroy(args params.Entities) (params.ErrorResults, error) {
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
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
		if canAccess(tag) {
			var unit *state.Unit
			unit, err = u.getUnit(tag)
			if err == nil {
				err = unit.Destroy()
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// DestroyAllSubordinates destroys all subordinates of each given unit.
func (u *UniterAPIV3) DestroyAllSubordinates(args params.Entities) (params.ErrorResults, error) {
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
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
		if canAccess(tag) {
			var unit *state.Unit
			unit, err = u.getUnit(tag)
			if err == nil {
				err = u.destroySubordinates(unit)
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// HasSubordinates returns the whether each given unit has any subordinates.
func (u *UniterAPIV3) HasSubordinates(args params.Entities) (params.BoolResults, error) {
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
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
		if canAccess(tag) {
			var unit *state.Unit
			unit, err = u.getUnit(tag)
			if err == nil {
				subordinates := unit.SubordinateNames()
				result.Results[i].Result = len(subordinates) > 0
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// CharmModifiedVersion returns the most CharmModifiedVersion for all given
// units or services.
func (u *UniterAPIV3) CharmModifiedVersion(args params.Entities) (params.IntResults, error) {
	results := params.IntResults{
		Results: make([]params.IntResult, len(args.Entities)),
	}

	accessUnitOrService := common.AuthEither(u.accessUnit, u.accessService)
	canAccess, err := accessUnitOrService()
	if err != nil {
		return results, err
	}
	for i, entity := range args.Entities {
		ver, err := u.charmModifiedVersion(entity.Tag, canAccess)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		results.Results[i].Result = ver
	}
	return results, nil
}

func (u *UniterAPIV3) charmModifiedVersion(tagStr string, canAccess func(names.Tag) bool) (int, error) {
	tag, err := names.ParseTag(tagStr)
	if err != nil {
		return -1, common.ErrPerm
	}
	if !canAccess(tag) {
		return -1, common.ErrPerm
	}
	unitOrService, err := u.st.FindEntity(tag)
	if err != nil {
		return -1, err
	}
	var service *state.Service
	switch entity := unitOrService.(type) {
	case *state.Service:
		service = entity
	case *state.Unit:
		service, err = entity.Service()
		if err != nil {
			return -1, err
		}
	default:
		return -1, errors.BadRequestf("type %t does not have a CharmModifiedVersion", entity)
	}
	return service.CharmModifiedVersion(), nil
}

// CharmURL returns the charm URL for all given units or services.
func (u *UniterAPIV3) CharmURL(args params.Entities) (params.StringBoolResults, error) {
	result := params.StringBoolResults{
		Results: make([]params.StringBoolResult, len(args.Entities)),
	}
	accessUnitOrService := common.AuthEither(u.accessUnit, u.accessService)
	canAccess, err := accessUnitOrService()
	if err != nil {
		return params.StringBoolResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
		if canAccess(tag) {
			var unitOrService state.Entity
			unitOrService, err = u.st.FindEntity(tag)
			if err == nil {
				charmURLer := unitOrService.(interface {
					CharmURL() (*charm.URL, bool)
				})
				curl, ok := charmURLer.CharmURL()
				if curl != nil {
					result.Results[i].Result = curl.String()
					result.Results[i].Ok = ok
				}
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// SetCharmURL sets the charm URL for each given unit. An error will
// be returned if a unit is dead, or the charm URL is not know.
func (u *UniterAPIV3) SetCharmURL(args params.EntitiesCharmURL) (params.ErrorResults, error) {
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
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
		if canAccess(tag) {
			var unit *state.Unit
			unit, err = u.getUnit(tag)
			if err == nil {
				var curl *charm.URL
				curl, err = charm.ParseURL(entity.CharmURL)
				if err == nil {
					err = unit.SetCharmURL(curl)
				}
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// OpenPorts sets the policy of the port range with protocol to be
// opened, for all given units.
func (u *UniterAPIV3) OpenPorts(args params.EntitiesPortRanges) (params.ErrorResults, error) {
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
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
		if canAccess(tag) {
			var unit *state.Unit
			unit, err = u.getUnit(tag)
			if err == nil {
				err = unit.OpenPorts(entity.Protocol, entity.FromPort, entity.ToPort)
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// ClosePorts sets the policy of the port range with protocol to be
// closed, for all given units.
func (u *UniterAPIV3) ClosePorts(args params.EntitiesPortRanges) (params.ErrorResults, error) {
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
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
		if canAccess(tag) {
			var unit *state.Unit
			unit, err = u.getUnit(tag)
			if err == nil {
				err = unit.ClosePorts(entity.Protocol, entity.FromPort, entity.ToPort)
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// WatchConfigSettings returns a NotifyWatcher for observing changes
// to each unit's service configuration settings. See also
// state/watcher.go:Unit.WatchConfigSettings().
func (u *UniterAPIV3) WatchConfigSettings(args params.Entities) (params.NotifyWatchResults, error) {
	result := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.NotifyWatchResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
		watcherId := ""
		if canAccess(tag) {
			watcherId, err = u.watchOneUnitConfigSettings(tag)
		}
		result.Results[i].NotifyWatcherId = watcherId
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// WatchActionNotifications returns a StringsWatcher for observing
// incoming action calls to a unit. See also state/watcher.go
// Unit.WatchActionNotifications(). This method is called from
// api/uniter/uniter.go WatchActionNotifications().
func (u *UniterAPIV3) WatchActionNotifications(args params.Entities) (params.StringsWatchResults, error) {
	nothing := params.StringsWatchResults{}

	result := params.StringsWatchResults{
		Results: make([]params.StringsWatchResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return nothing, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			return nothing, err
		}
		err = common.ErrPerm
		if canAccess(tag) {
			result.Results[i], err = u.watchOneUnitActionNotifications(tag)
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// ConfigSettings returns the complete set of service charm config
// settings available to each given unit.
func (u *UniterAPIV3) ConfigSettings(args params.Entities) (params.ConfigSettingsResults, error) {
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
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
		if canAccess(tag) {
			var unit *state.Unit
			unit, err = u.getUnit(tag)
			if err == nil {
				var settings charm.Settings
				settings, err = unit.ConfigSettings()
				if err == nil {
					result.Results[i].Settings = params.ConfigSettings(settings)
				}
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// WatchServiceRelations returns a StringsWatcher, for each given
// service, that notifies of changes to the lifecycles of relations
// involving that service.
func (u *UniterAPIV3) WatchServiceRelations(args params.Entities) (params.StringsWatchResults, error) {
	result := params.StringsWatchResults{
		Results: make([]params.StringsWatchResult, len(args.Entities)),
	}
	canAccess, err := u.accessService()
	if err != nil {
		return params.StringsWatchResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseServiceTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
		if canAccess(tag) {
			result.Results[i], err = u.watchOneServiceRelations(tag)
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// CharmArchiveSha256 returns the SHA256 digest of the charm archive
// (bundle) data for each charm url in the given parameters.
func (u *UniterAPIV3) CharmArchiveSha256(args params.CharmURLs) (params.StringResults, error) {
	result := params.StringResults{
		Results: make([]params.StringResult, len(args.URLs)),
	}
	for i, arg := range args.URLs {
		curl, err := charm.ParseURL(arg.URL)
		if err != nil {
			err = common.ErrPerm
		} else {
			var sch *state.Charm
			sch, err = u.st.Charm(curl)
			if errors.IsNotFound(err) {
				err = common.ErrPerm
			}
			if err == nil {
				result.Results[i].Result = sch.BundleSha256()
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// CharmArchiveURLs returns the URLS for the charm archive
// (bundle) data for each charm url in the given parameters.
func (u *UniterAPIV3) CharmArchiveURLs(args params.CharmURLs) (params.StringsResults, error) {
	apiHostPorts, err := u.st.APIHostPorts()
	if err != nil {
		return params.StringsResults{}, err
	}
	modelUUID := u.st.ModelUUID()
	result := params.StringsResults{
		Results: make([]params.StringsResult, len(args.URLs)),
	}
	for i, curl := range args.URLs {
		if _, err := charm.ParseURL(curl.URL); err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		urlPath := "/"
		if modelUUID != "" {
			urlPath = path.Join(urlPath, "model", modelUUID)
		}
		urlPath = path.Join(urlPath, "charms")
		archiveURLs := make([]string, len(apiHostPorts))
		for j, server := range apiHostPorts {
			archiveURL := &url.URL{
				Scheme: "https",
				Host:   network.SelectInternalHostPort(server, false),
				Path:   urlPath,
			}
			q := archiveURL.Query()
			q.Set("url", curl.URL)
			q.Set("file", "*")
			archiveURL.RawQuery = q.Encode()
			archiveURLs[j] = archiveURL.String()
		}
		result.Results[i].Result = archiveURLs
	}
	return result, nil
}

// Relation returns information about all given relation/unit pairs,
// including their id, key and the local endpoint.
func (u *UniterAPIV3) Relation(args params.RelationUnits) (params.RelationResults, error) {
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
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// Actions returns the Actions by Tags passed and ensures that the Unit asking
// for them is the same Unit that has the Actions.
func (u *UniterAPIV3) Actions(args params.Entities) (params.ActionsQueryResults, error) {
	nothing := params.ActionsQueryResults{}

	actionFn, err := u.authAndActionFromTagFn()
	if err != nil {
		return nothing, err
	}

	results := params.ActionsQueryResults{
		Results: make([]params.ActionsQueryResult, len(args.Entities)),
	}

	for i, arg := range args.Entities {
		action, err := actionFn(arg.Tag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		if action.Status() != state.ActionPending {
			results.Results[i].Error = common.ServerError(common.ErrActionNotAvailable)
			continue
		}
		results.Results[i].Action.Action = &params.Action{
			Name:       action.Name(),
			Parameters: action.Parameters(),
		}
	}

	return results, nil
}

// BeginActions marks the actions represented by the passed in Tags as running.
func (u *UniterAPIV3) BeginActions(args params.Entities) (params.ErrorResults, error) {
	nothing := params.ErrorResults{}

	actionFn, err := u.authAndActionFromTagFn()
	if err != nil {
		return nothing, err
	}

	results := params.ErrorResults{Results: make([]params.ErrorResult, len(args.Entities))}

	for i, arg := range args.Entities {
		action, err := actionFn(arg.Tag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}

		_, err = action.Begin()
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
	}

	return results, nil
}

// FinishActions saves the result of a completed Action
func (u *UniterAPIV3) FinishActions(args params.ActionExecutionResults) (params.ErrorResults, error) {
	nothing := params.ErrorResults{}

	actionFn, err := u.authAndActionFromTagFn()
	if err != nil {
		return nothing, err
	}

	results := params.ErrorResults{Results: make([]params.ErrorResult, len(args.Results))}

	for i, arg := range args.Results {
		action, err := actionFn(arg.ActionTag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		actionResults, err := paramsActionExecutionResultsToStateActionResults(arg)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}

		_, err = action.Finish(actionResults)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
	}

	return results, nil
}

// paramsActionExecutionResultsToStateActionResults does exactly what
// the name implies.
func paramsActionExecutionResultsToStateActionResults(arg params.ActionExecutionResult) (state.ActionResults, error) {
	var status state.ActionStatus
	switch arg.Status {
	case params.ActionCancelled:
		status = state.ActionCancelled
	case params.ActionCompleted:
		status = state.ActionCompleted
	case params.ActionFailed:
		status = state.ActionFailed
	case params.ActionPending:
		status = state.ActionPending
	default:
		return state.ActionResults{}, errors.Errorf("unrecognized action status '%s'", arg.Status)
	}
	return state.ActionResults{
		Status:  status,
		Results: arg.Results,
		Message: arg.Message,
	}, nil
}

// RelationById returns information about all given relations,
// specified by their ids, including their key and the local
// endpoint.
func (u *UniterAPIV3) RelationById(args params.RelationIds) (params.RelationResults, error) {
	result := params.RelationResults{
		Results: make([]params.RelationResult, len(args.RelationIds)),
	}
	for i, relId := range args.RelationIds {
		relParams, err := u.getOneRelationById(relId)
		if err == nil {
			result.Results[i] = relParams
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// JoinedRelations returns the tags of all relations for which each supplied unit
// has entered scope. It should be called RelationsInScope, but it's not convenient
// to make that change until we have versioned APIs.
func (u *UniterAPIV3) JoinedRelations(args params.Entities) (params.StringsResults, error) {
	result := params.StringsResults{
		Results: make([]params.StringsResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	canRead, err := u.accessUnit()
	if err != nil {
		return params.StringsResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
		if canRead(tag) {
			var unit *state.Unit
			unit, err = u.getUnit(tag)
			if err == nil {
				result.Results[i].Result, err = relationsInScopeTags(unit)
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// CurrentModel returns the name and UUID for the current juju model.
func (u *UniterAPIV3) CurrentModel() (params.ModelResult, error) {
	result := params.ModelResult{}
	env, err := u.st.Model()
	if err == nil {
		result.Name = env.Name()
		result.UUID = env.UUID()
	}
	return result, err
}

// ProviderType returns the provider type used by the current juju
// model.
//
// TODO(dimitern): Refactor the uniter to call this instead of calling
// ModelConfig() just to get the provider type. Once we have machine
// addresses, this might be completely unnecessary though.
func (u *UniterAPIV3) ProviderType() (params.StringResult, error) {
	result := params.StringResult{}
	cfg, err := u.st.ModelConfig()
	if err == nil {
		result.Result = cfg.Type()
	}
	return result, err
}

// EnterScope ensures each unit has entered its scope in the relation,
// for all of the given relation/unit pairs. See also
// state.RelationUnit.EnterScope().
func (u *UniterAPIV3) EnterScope(args params.RelationUnits) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.RelationUnits)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.ErrorResults{}, err
	}
	for i, arg := range args.RelationUnits {
		tag, err := names.ParseUnitTag(arg.Unit)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		relUnit, err := u.getRelationUnit(canAccess, arg.Relation, tag)
		if err == nil {
			// Construct the settings, passing the unit's
			// private address (we already know it).
			privateAddress, _ := relUnit.PrivateAddress()
			settings := map[string]interface{}{
				"private-address": privateAddress.Value,
			}
			err = relUnit.EnterScope(settings)
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// LeaveScope signals each unit has left its scope in the relation,
// for all of the given relation/unit pairs. See also
// state.RelationUnit.LeaveScope().
func (u *UniterAPIV3) LeaveScope(args params.RelationUnits) (params.ErrorResults, error) {
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
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		relUnit, err := u.getRelationUnit(canAccess, arg.Relation, unit)
		if err == nil {
			err = relUnit.LeaveScope()
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// ReadSettings returns the local settings of each given set of
// relation/unit.
func (u *UniterAPIV3) ReadSettings(args params.RelationUnits) (params.SettingsResults, error) {
	result := params.SettingsResults{
		Results: make([]params.SettingsResult, len(args.RelationUnits)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.SettingsResults{}, err
	}
	for i, arg := range args.RelationUnits {
		unit, err := names.ParseUnitTag(arg.Unit)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		relUnit, err := u.getRelationUnit(canAccess, arg.Relation, unit)
		if err == nil {
			var settings *state.Settings
			settings, err = relUnit.Settings()
			if err == nil {
				result.Results[i].Settings, err = convertRelationSettings(settings.Map())
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// ReadRemoteSettings returns the remote settings of each given set of
// relation/local unit/remote unit.
func (u *UniterAPIV3) ReadRemoteSettings(args params.RelationUnitPairs) (params.SettingsResults, error) {
	result := params.SettingsResults{
		Results: make([]params.SettingsResult, len(args.RelationUnitPairs)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.SettingsResults{}, err
	}
	for i, arg := range args.RelationUnitPairs {
		unit, err := names.ParseUnitTag(arg.LocalUnit)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		relUnit, err := u.getRelationUnit(canAccess, arg.Relation, unit)
		if err == nil {
			// TODO(dfc) rework this logic
			remoteUnit := ""
			remoteUnit, err = u.checkRemoteUnit(relUnit, arg.RemoteUnit)
			if err == nil {
				var settings map[string]interface{}
				settings, err = relUnit.ReadSettings(remoteUnit)
				if err == nil {
					result.Results[i].Settings, err = convertRelationSettings(settings)
				}
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// UpdateSettings persists all changes made to the local settings of
// all given pairs of relation and unit. Keys with empty values are
// considered a signal to delete these values.
func (u *UniterAPIV3) UpdateSettings(args params.RelationUnitsSettings) (params.ErrorResults, error) {
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
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		relUnit, err := u.getRelationUnit(canAccess, arg.Relation, unit)
		if err == nil {
			var settings *state.Settings
			settings, err = relUnit.Settings()
			if err == nil {
				for k, v := range arg.Settings {
					if v == "" {
						settings.Delete(k)
					} else {
						settings.Set(k, v)
					}
				}
				_, err = settings.Write()
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// WatchRelationUnits returns a RelationUnitsWatcher for observing
// changes to every unit in the supplied relation that is visible to
// the supplied unit. See also state/watcher.go:RelationUnit.Watch().
func (u *UniterAPIV3) WatchRelationUnits(args params.RelationUnits) (params.RelationUnitsWatchResults, error) {
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
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		relUnit, err := u.getRelationUnit(canAccess, arg.Relation, unit)
		if err == nil {
			result.Results[i], err = u.watchOneRelationUnit(relUnit)
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// WatchUnitAddresses returns a NotifyWatcher for observing changes
// to each unit's addresses.
func (u *UniterAPIV3) WatchUnitAddresses(args params.Entities) (params.NotifyWatchResults, error) {
	result := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.NotifyWatchResults{}, err
	}
	for i, entity := range args.Entities {
		unit, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
		watcherId := ""
		if canAccess(unit) {
			watcherId, err = u.watchOneUnitAddresses(unit)
		}
		result.Results[i].NotifyWatcherId = watcherId
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

func (u *UniterAPIV3) getUnit(tag names.UnitTag) (*state.Unit, error) {
	return u.st.Unit(tag.Id())
}

func (u *UniterAPIV3) getService(tag names.ServiceTag) (*state.Service, error) {
	return u.st.Service(tag.Id())
}

func (u *UniterAPIV3) getRelationUnit(canAccess common.AuthFunc, relTag string, unitTag names.UnitTag) (*state.RelationUnit, error) {
	rel, unit, err := u.getRelationAndUnit(canAccess, relTag, unitTag)
	if err != nil {
		return nil, err
	}
	return rel.Unit(unit)
}

func (u *UniterAPIV3) getOneRelationById(relId int) (params.RelationResult, error) {
	nothing := params.RelationResult{}
	rel, err := u.st.Relation(relId)
	if errors.IsNotFound(err) {
		return nothing, common.ErrPerm
	} else if err != nil {
		return nothing, err
	}
	tag := u.auth.GetAuthTag()
	switch tag.(type) {
	case names.UnitTag:
		// do nothing
	default:
		panic("authenticated entity is not a unit")
	}
	unit, err := u.st.FindEntity(tag)
	if err != nil {
		return nothing, err
	}
	// Use the currently authenticated unit to get the endpoint.
	result, err := u.prepareRelationResult(rel, unit.(*state.Unit))
	if err != nil {
		// An error from prepareRelationResult means the authenticated
		// unit's service is not part of the requested
		// relation. That's why it's appropriate to return ErrPerm
		// here.
		return nothing, common.ErrPerm
	}
	return result, nil
}

func (u *UniterAPIV3) getRelationAndUnit(canAccess common.AuthFunc, relTag string, unitTag names.UnitTag) (*state.Relation, *state.Unit, error) {
	tag, err := names.ParseRelationTag(relTag)
	if err != nil {
		return nil, nil, common.ErrPerm
	}
	rel, err := u.st.KeyRelation(tag.Id())
	if errors.IsNotFound(err) {
		return nil, nil, common.ErrPerm
	} else if err != nil {
		return nil, nil, err
	}
	if !canAccess(unitTag) {
		return nil, nil, common.ErrPerm
	}
	unit, err := u.getUnit(unitTag)
	return rel, unit, err
}

func (u *UniterAPIV3) prepareRelationResult(rel *state.Relation, unit *state.Unit) (params.RelationResult, error) {
	nothing := params.RelationResult{}
	ep, err := rel.Endpoint(unit.ServiceName())
	if err != nil {
		// An error here means the unit's service is not part of the
		// relation.
		return nothing, err
	}
	return params.RelationResult{
		Id:   rel.Id(),
		Key:  rel.String(),
		Life: params.Life(rel.Life().String()),
		Endpoint: multiwatcher.Endpoint{
			ServiceName: ep.ServiceName,
			Relation:    ep.Relation,
		},
	}, nil
}

func (u *UniterAPIV3) getOneRelation(canAccess common.AuthFunc, relTag, unitTag string) (params.RelationResult, error) {
	nothing := params.RelationResult{}
	tag, err := names.ParseUnitTag(unitTag)
	if err != nil {
		return nothing, common.ErrPerm
	}
	rel, unit, err := u.getRelationAndUnit(canAccess, relTag, tag)
	if err != nil {
		return nothing, err
	}
	return u.prepareRelationResult(rel, unit)
}

func (u *UniterAPIV3) destroySubordinates(principal *state.Unit) error {
	subordinates := principal.SubordinateNames()
	for _, subName := range subordinates {
		unit, err := u.getUnit(names.NewUnitTag(subName))
		if err != nil {
			return err
		}
		if err = unit.Destroy(); err != nil {
			return err
		}
	}
	return nil
}

func (u *UniterAPIV3) watchOneServiceRelations(tag names.ServiceTag) (params.StringsWatchResult, error) {
	nothing := params.StringsWatchResult{}
	service, err := u.getService(tag)
	if err != nil {
		return nothing, err
	}
	watch := service.WatchRelations()
	// Consume the initial event and forward it to the result.
	if changes, ok := <-watch.Changes(); ok {
		return params.StringsWatchResult{
			StringsWatcherId: u.resources.Register(watch),
			Changes:          changes,
		}, nil
	}
	return nothing, watcher.EnsureErr(watch)
}

func (u *UniterAPIV3) watchOneUnitConfigSettings(tag names.UnitTag) (string, error) {
	unit, err := u.getUnit(tag)
	if err != nil {
		return "", err
	}
	watch, err := unit.WatchConfigSettings()
	if err != nil {
		return "", err
	}
	// Consume the initial event. Technically, API
	// calls to Watch 'transmit' the initial event
	// in the Watch response. But NotifyWatchers
	// have no state to transmit.
	if _, ok := <-watch.Changes(); ok {
		return u.resources.Register(watch), nil
	}
	return "", watcher.EnsureErr(watch)
}

func (u *UniterAPIV3) watchOneUnitActionNotifications(tag names.UnitTag) (params.StringsWatchResult, error) {
	nothing := params.StringsWatchResult{}
	unit, err := u.getUnit(tag)
	if err != nil {
		return nothing, err
	}
	watch := unit.WatchActionNotifications()

	if changes, ok := <-watch.Changes(); ok {
		return params.StringsWatchResult{
			StringsWatcherId: u.resources.Register(watch),
			Changes:          changes,
		}, nil
	}
	return nothing, watcher.EnsureErr(watch)
}

func (u *UniterAPIV3) watchOneUnitAddresses(tag names.UnitTag) (string, error) {
	unit, err := u.getUnit(tag)
	if err != nil {
		return "", err
	}
	machineId, err := unit.AssignedMachineId()
	if err != nil {
		return "", err
	}
	machine, err := u.st.Machine(machineId)
	if err != nil {
		return "", err
	}
	watch := machine.WatchAddresses()
	// Consume the initial event. Technically, API
	// calls to Watch 'transmit' the initial event
	// in the Watch response. But NotifyWatchers
	// have no state to transmit.
	if _, ok := <-watch.Changes(); ok {
		return u.resources.Register(watch), nil
	}
	return "", watcher.EnsureErr(watch)
}

func (u *UniterAPIV3) watchOneRelationUnit(relUnit *state.RelationUnit) (params.RelationUnitsWatchResult, error) {
	watch := relUnit.Watch()
	// Consume the initial event and forward it to the result.
	if changes, ok := <-watch.Changes(); ok {
		return params.RelationUnitsWatchResult{
			RelationUnitsWatcherId: u.resources.Register(watch),
			Changes:                changes,
		}, nil
	}
	return params.RelationUnitsWatchResult{}, watcher.EnsureErr(watch)
}

func (u *UniterAPIV3) checkRemoteUnit(relUnit *state.RelationUnit, remoteUnitTag string) (string, error) {
	// Make sure the unit is indeed remote.
	if remoteUnitTag == u.auth.GetAuthTag().String() {
		return "", common.ErrPerm
	}
	// Check remoteUnit is indeed related. Note that we don't want to actually get
	// the *Unit, because it might have been removed; but its relation settings will
	// persist until the relation itself has been removed (and must remain accessible
	// because the local unit's view of reality may be time-shifted).
	tag, err := names.ParseUnitTag(remoteUnitTag)
	if err != nil {
		return "", common.ErrPerm
	}
	remoteUnitName := tag.Id()
	remoteServiceName, err := names.UnitService(remoteUnitName)
	if err != nil {
		return "", common.ErrPerm
	}
	rel := relUnit.Relation()
	_, err = rel.RelatedEndpoints(remoteServiceName)
	if err != nil {
		return "", common.ErrPerm
	}
	return remoteUnitName, nil
}

// authAndActionFromTagFn first authenticates the request, and then returns
// a function with which to authenticate and retrieve each action in the
// request.
func (u *UniterAPIV3) authAndActionFromTagFn() (func(string) (*state.Action, error), error) {
	canAccess, err := u.accessUnit()
	if err != nil {
		return nil, err
	}
	unit, ok := u.auth.GetAuthTag().(names.UnitTag)
	if !ok {
		return nil, fmt.Errorf("calling entity is not a unit")
	}

	return func(tag string) (*state.Action, error) {
		actionTag, err := names.ParseActionTag(tag)
		if err != nil {
			return nil, err
		}
		action, err := u.st.ActionByTag(actionTag)
		if err != nil {
			return nil, err
		}
		receiverTag, err := names.ActionReceiverTag(action.Receiver())
		if err != nil {
			return nil, err
		}
		if unit != receiverTag {
			return nil, common.ErrPerm
		}

		if !canAccess(receiverTag) {
			return nil, common.ErrPerm
		}
		return action, nil
	}, nil
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

func relationsInScopeTags(unit *state.Unit) ([]string, error) {
	relations, err := unit.RelationsInScope()
	if err != nil {
		return nil, err
	}
	tags := make([]string, len(relations))
	for i, relation := range relations {
		tags[i] = relation.Tag().String()
	}
	return tags, nil
}

func leadershipSettingsAccessorFactory(
	st *state.State,
	resources *common.Resources,
	auth common.Authorizer,
) *leadershipapiserver.LeadershipSettingsAccessor {
	registerWatcher := func(serviceId string) (string, error) {
		service, err := st.Service(serviceId)
		if err != nil {
			return "", err
		}
		w := service.WatchLeaderSettings()
		if _, ok := <-w.Changes(); ok {
			return resources.Register(w), nil
		}
		return "", watcher.EnsureErr(w)
	}
	getSettings := func(serviceId string) (map[string]string, error) {
		service, err := st.Service(serviceId)
		if err != nil {
			return nil, err
		}
		return service.LeaderSettings()
	}
	writeSettings := func(token leadership.Token, serviceId string, settings map[string]string) error {
		service, err := st.Service(serviceId)
		if err != nil {
			return err
		}
		return service.UpdateLeaderSettings(token, settings)
	}
	return leadershipapiserver.NewLeadershipSettingsAccessor(
		auth,
		registerWatcher,
		getSettings,
		st.LeadershipChecker().LeadershipCheck,
		writeSettings,
	)
}

// AddMetricBatches adds the metrics for the specified unit.
func (u *UniterAPIV3) AddMetricBatches(args params.MetricBatchParams) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Batches)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		logger.Warningf("failed to check unit access: %v", err)
		return params.ErrorResults{}, common.ErrPerm
	}
	for i, batch := range args.Batches {
		tag, err := names.ParseUnitTag(batch.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		if !canAccess(tag) {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		metrics := make([]state.Metric, len(batch.Batch.Metrics))
		for j, metric := range batch.Batch.Metrics {
			metrics[j] = state.Metric{
				Key:   metric.Key,
				Value: metric.Value,
				Time:  metric.Time,
			}
		}
		_, err = u.st.AddMetrics(state.BatchParam{
			UUID:     batch.Batch.UUID,
			Created:  batch.Batch.Created,
			CharmURL: batch.Batch.CharmURL,
			Metrics:  metrics,
			Unit:     tag,
		})
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// NetworkConfig returns information about all given relation/unit pairs,
// including their id, key and the local endpoint.
func (u *UniterAPIV3) NetworkConfig(args params.RelationUnits) (params.UnitNetworkConfigResults, error) {
	result := params.UnitNetworkConfigResults{
		Results: make([]params.UnitNetworkConfigResult, len(args.RelationUnits)),
	}

	canAccess, err := u.accessUnit()
	if err != nil {
		return params.UnitNetworkConfigResults{}, err
	}

	for i, rel := range args.RelationUnits {
		netConfig, err := u.getOneNetworkConfig(canAccess, rel.Relation, rel.Unit)
		if err == nil {
			result.Results[i].Error = nil
			result.Results[i].Config = netConfig
		} else {
			result.Results[i].Error = common.ServerError(err)
		}
	}
	return result, nil
}

func (u *UniterAPIV3) getOneNetworkConfig(canAccess common.AuthFunc, tagRel, tagUnit string) ([]params.NetworkConfig, error) {
	unitTag, err := names.ParseUnitTag(tagUnit)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if !canAccess(unitTag) {
		return nil, common.ErrPerm
	}

	relTag, err := names.ParseRelationTag(tagRel)
	if err != nil {
		return nil, errors.Trace(err)
	}

	unit, err := u.getUnit(unitTag)
	if err != nil {
		return nil, errors.Trace(err)
	}

	service, err := unit.Service()
	if err != nil {
		return nil, errors.Trace(err)
	}

	bindings, err := service.EndpointBindings()
	if err != nil {
		return nil, errors.Trace(err)
	}

	rel, err := u.st.KeyRelation(relTag.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}

	endpoint, err := rel.Endpoint(service.Name())
	if err != nil {
		return nil, errors.Trace(err)
	}

	machineID, err := unit.AssignedMachineId()
	if err != nil {
		return nil, errors.Trace(err)
	}

	machine, err := u.st.Machine(machineID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var results []params.NetworkConfig

	boundSpace, isBound := bindings[endpoint.Name]
	if !isBound || boundSpace == "" {
		name := endpoint.Name
		logger.Debugf("endpoint %q not explicitly bound to a space, using preferred private address for machine %q", name, machineID)

		privateAddress, err := machine.PrivateAddress()
		if err != nil && !network.IsNoAddress(err) {
			return nil, errors.Annotatef(err, "getting machine %q preferred private address", machineID)
		}

		results = append(results, params.NetworkConfig{
			Address: privateAddress.Value,
		})
		return results, nil
	}
	logger.Debugf("endpoint %q is explicitly bound to space %q", endpoint.Name, boundSpace)

	// TODO(dimitern): Use NetworkInterfaces() instead later, this is just for
	// the PoC to enable minimal network-get implementation returning just the
	// primary address.
	//
	// LKK Card: https://canonical.leankit.com/Boards/View/101652562/119258804
	addresses := machine.ProviderAddresses()
	logger.Infof(
		"geting network config for machine %q with addresses %+v, hosting unit %q of service %q, with bindings %+v",
		machineID, addresses, unit.Name(), service.Name(), bindings,
	)

	for _, addr := range addresses {
		space := string(addr.SpaceName)
		if space != boundSpace {
			logger.Debugf("skipping address %q: want bound to space %q, got space %q", addr.Value, boundSpace, space)
			continue
		}
		logger.Debugf("endpoint %q bound to space %q has address %q", endpoint.Name, boundSpace, addr.Value)

		// TODO(dimitern): Fill in the rest later (see linked LKK card above).
		results = append(results, params.NetworkConfig{
			Address: addr.Value,
		})
	}

	return results, nil
}
