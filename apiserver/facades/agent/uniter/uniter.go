// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package uniter implements the API interface used by the uniter worker.

package uniter

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/firewall"
	"github.com/juju/juju/apiserver/common/networkingcommon"
	"github.com/juju/juju/apiserver/facade"
	leadershipapiserver "github.com/juju/juju/apiserver/facades/agent/leadership"
	"github.com/juju/juju/apiserver/facades/agent/meterstatus"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/status"
	"github.com/juju/utils/set"
)

var logger = loggo.GetLogger("juju.apiserver.uniter")

// UniterAPI implements the latest version (v7) of the Uniter API.
type UniterAPI struct {
	*common.LifeGetter
	*StatusAPI
	*common.DeadEnsurer
	*common.AgentEntityWatcher
	*common.APIAddresser
	*common.ModelWatcher
	*common.RebootRequester
	*leadershipapiserver.LeadershipSettingsAccessor
	meterstatus.MeterStatus
	m                 *state.Model
	st                *state.State
	auth              facade.Authorizer
	resources         facade.Resources
	accessUnit        common.GetAuthFunc
	accessApplication common.GetAuthFunc
	unit              *state.Unit
	accessMachine     common.GetAuthFunc
	StorageAPI
}

// UniterAPIV6 adds NetworkInfo as a preferred method to calling NetworkConfig.
type UniterAPIV6 struct {
	UniterAPI
}

// UniterAPIV5 returns a RelationResultsV5 instead of RelationResults
// from Relation and RelationById - elements don't have an
// OtherApplication field.
type UniterAPIV5 struct {
	UniterAPIV6
}

// UniterAPIV4 has old WatchApplicationRelations and NetworkConfig
// methods, and doesn't have the new SLALevel, NetworkInfo or
// WatchUnitRelations methods.
type UniterAPIV4 struct {
	UniterAPIV5
}

// NewUniterAPI creates a new instance of the core Uniter API.
func NewUniterAPI(st *state.State, resources facade.Resources, authorizer facade.Authorizer) (*UniterAPI, error) {
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
	accessApplication := func() (common.AuthFunc, error) {
		switch tag := authorizer.GetAuthTag().(type) {
		case names.UnitTag:
			entity, err := st.Unit(tag.Id())
			if err != nil {
				return nil, errors.Trace(err)
			}
			applicationName := entity.ApplicationName()
			applicationTag := names.NewApplicationTag(applicationName)
			return func(tag names.Tag) bool {
				return tag == applicationTag
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
	ss, err := getStorageState(st)
	if err != nil {
		return nil, errors.Annotate(err, "getting state")
	}
	storageAPI, err := newStorageAPI(ss, resources, accessUnit)
	if err != nil {
		return nil, err
	}
	msAPI, err := meterstatus.NewMeterStatusAPI(st, resources, authorizer)
	if err != nil {
		return nil, errors.Annotate(err, "could not create meter status API handler")
	}
	accessUnitOrApplication := common.AuthAny(accessUnit, accessApplication)

	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &UniterAPI{
		LifeGetter:                 common.NewLifeGetter(st, accessUnitOrApplication),
		DeadEnsurer:                common.NewDeadEnsurer(st, accessUnit),
		AgentEntityWatcher:         common.NewAgentEntityWatcher(st, resources, accessUnitOrApplication),
		APIAddresser:               common.NewAPIAddresser(st, resources),
		ModelWatcher:               common.NewModelWatcher(m, resources, authorizer),
		RebootRequester:            common.NewRebootRequester(st, accessMachine),
		LeadershipSettingsAccessor: leadershipSettingsAccessorFactory(st, resources, authorizer),
		MeterStatus:                msAPI,
		// TODO(fwereade): so *every* unit should be allowed to get/set its
		// own status *and* its application's? This is not a pleasing arrangement.
		StatusAPI: NewStatusAPI(st, accessUnitOrApplication),

		st:                st,
		m:                 m,
		auth:              authorizer,
		resources:         resources,
		accessUnit:        accessUnit,
		accessApplication: accessApplication,
		accessMachine:     accessMachine,
		unit:              unit,
		StorageAPI:        *storageAPI,
	}, nil
}

// NewUniterAPIV6 creates an instance of the V6 uniter API.
func NewUniterAPIV6(st *state.State, resources facade.Resources, authorizer facade.Authorizer) (*UniterAPIV6, error) {
	uniterAPI, err := NewUniterAPI(st, resources, authorizer)
	if err != nil {
		return nil, err
	}
	return &UniterAPIV6{
		UniterAPI: *uniterAPI,
	}, nil
}

// NewUniterAPIV5 creates an instance of the V5 uniter API.
func NewUniterAPIV5(st *state.State, resources facade.Resources, authorizer facade.Authorizer) (*UniterAPIV5, error) {
	uniterAPI, err := NewUniterAPIV6(st, resources, authorizer)
	if err != nil {
		return nil, err
	}
	return &UniterAPIV5{
		UniterAPIV6: *uniterAPI,
	}, nil
}

// NewUniterAPIV4 creates an instance of the V4 uniter API.
func NewUniterAPIV4(st *state.State, resources facade.Resources, authorizer facade.Authorizer) (*UniterAPIV4, error) {
	uniterAPI, err := NewUniterAPIV5(st, resources, authorizer)
	if err != nil {
		return nil, err
	}
	return &UniterAPIV4{
		UniterAPIV5: *uniterAPI,
	}, nil
}

// AllMachinePorts returns all opened port ranges for each given
// machine (on all networks).
func (u *UniterAPI) AllMachinePorts(args params.Entities) (params.MachinePortsResults, error) {
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

// AssignedMachine returns the machine tag for each given unit tag, or
// an error satisfying params.IsCodeNotAssigned when a unit has no
// assigned machine.
func (u *UniterAPI) AssignedMachine(args params.Entities) (params.StringResults, error) {
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

func (u *UniterAPI) getMachine(tag names.MachineTag) (*state.Machine, error) {
	return u.st.Machine(tag.Id())
}

func (u *UniterAPI) getOneMachinePorts(canAccess common.AuthFunc, machineTag string) params.MachinePortsResult {
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
func (u *UniterAPI) PublicAddress(args params.Entities) (params.StringResults, error) {
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
				} else if network.IsNoAddressError(err) {
					err = common.NoAddressSetError(tag, "public")
				}
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// PrivateAddress returns the private address for each given unit, if set.
func (u *UniterAPI) PrivateAddress(args params.Entities) (params.StringResults, error) {
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
				} else if network.IsNoAddressError(err) {
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
func (u *UniterAPI) AvailabilityZone(args params.Entities) (params.StringResults, error) {
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
func (u *UniterAPI) Resolved(args params.Entities) (params.ResolvedModeResults, error) {
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
func (u *UniterAPI) ClearResolved(args params.Entities) (params.ErrorResults, error) {
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
func (u *UniterAPI) GetPrincipal(args params.Entities) (params.StringBoolResults, error) {
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
func (u *UniterAPI) Destroy(args params.Entities) (params.ErrorResults, error) {
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
func (u *UniterAPI) DestroyAllSubordinates(args params.Entities) (params.ErrorResults, error) {
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
func (u *UniterAPI) HasSubordinates(args params.Entities) (params.BoolResults, error) {
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
// units or applications.
func (u *UniterAPI) CharmModifiedVersion(args params.Entities) (params.IntResults, error) {
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
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		results.Results[i].Result = ver
	}
	return results, nil
}

func (u *UniterAPI) charmModifiedVersion(tagStr string, canAccess func(names.Tag) bool) (int, error) {
	tag, err := names.ParseTag(tagStr)
	if err != nil {
		return -1, common.ErrPerm
	}
	if !canAccess(tag) {
		return -1, common.ErrPerm
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
func (u *UniterAPI) CharmURL(args params.Entities) (params.StringBoolResults, error) {
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
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
		if canAccess(tag) {
			var unitOrApplication state.Entity
			unitOrApplication, err = u.st.FindEntity(tag)
			if err == nil {
				charmURLer := unitOrApplication.(interface {
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
func (u *UniterAPI) SetCharmURL(args params.EntitiesCharmURL) (params.ErrorResults, error) {
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

// WorkloadVersion returns the workload version for all given units or applications.
func (u *UniterAPI) WorkloadVersion(args params.Entities) (params.StringResults, error) {
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
			resultItem.Error = common.ServerError(err)
			continue
		}
		if !canAccess(tag) {
			resultItem.Error = common.ServerError(common.ErrPerm)
			continue
		}
		unit, err := u.getUnit(tag)
		if err != nil {
			resultItem.Error = common.ServerError(err)
			continue
		}
		version, err := unit.WorkloadVersion()
		if err != nil {
			resultItem.Error = common.ServerError(err)
			continue
		}
		resultItem.Result = version
	}
	return result, nil
}

// SetWorkloadVersion sets the workload version for each given unit. An error will
// be returned if a unit is dead.
func (u *UniterAPI) SetWorkloadVersion(args params.EntityWorkloadVersions) (params.ErrorResults, error) {
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
			resultItem.Error = common.ServerError(err)
			continue
		}
		if !canAccess(tag) {
			resultItem.Error = common.ServerError(common.ErrPerm)
			continue
		}
		unit, err := u.getUnit(tag)
		if err != nil {
			resultItem.Error = common.ServerError(err)
			continue
		}
		err = unit.SetWorkloadVersion(entity.WorkloadVersion)
		if err != nil {
			resultItem.Error = common.ServerError(err)
		}
	}
	return result, nil
}

// OpenPorts sets the policy of the port range with protocol to be
// opened, for all given units.
func (u *UniterAPI) OpenPorts(args params.EntitiesPortRanges) (params.ErrorResults, error) {
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
func (u *UniterAPI) ClosePorts(args params.EntitiesPortRanges) (params.ErrorResults, error) {
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
// to each unit's application configuration settings. See also
// state/watcher.go:Unit.WatchConfigSettings().
func (u *UniterAPI) WatchConfigSettings(args params.Entities) (params.NotifyWatchResults, error) {
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
func (u *UniterAPI) WatchActionNotifications(args params.Entities) (params.StringsWatchResults, error) {
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
func (u *UniterAPI) ConfigSettings(args params.Entities) (params.ConfigSettingsResults, error) {
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

// CharmArchiveSha256 returns the SHA256 digest of the charm archive
// (bundle) data for each charm url in the given parameters.
func (u *UniterAPI) CharmArchiveSha256(args params.CharmURLs) (params.StringResults, error) {
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

// Relation returns information about all given relation/unit pairs,
// including their id, key and the local endpoint.
func (u *UniterAPI) Relation(args params.RelationUnits) (params.RelationResults, error) {
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
func (u *UniterAPI) Actions(args params.Entities) (params.ActionResults, error) {
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
func (u *UniterAPI) BeginActions(args params.Entities) (params.ErrorResults, error) {
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
func (u *UniterAPI) FinishActions(args params.ActionExecutionResults) (params.ErrorResults, error) {
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

// RelationById returns information about all given relations,
// specified by their ids, including their key and the local
// endpoint.
func (u *UniterAPI) RelationById(args params.RelationIds) (params.RelationResults, error) {
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
// has entered scope.
// TODO(wallyworld) - this API is replaced by RelationsStatus
func (u *UniterAPIV6) JoinedRelations(args params.Entities) (params.StringsResults, error) {
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

// RelationsStatus returns for each unit the corresponding relation and status information.
func (u *UniterAPI) RelationsStatus(args params.Entities) (params.RelationUnitStatusResults, error) {
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
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
		if canRead(tag) {
			var unit *state.Unit
			unit, err = u.getUnit(tag)
			if err == nil {
				result.Results[i].RelationResults, err = relationResults(unit)
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// Refresh retrieves the latest values for attributes on this unit.
func (u *UniterAPI) Refresh(args params.Entities) (params.UnitRefreshResults, error) {
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
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
		if canRead(tag) {
			var unit *state.Unit
			if unit, err = u.getUnit(tag); err == nil {
				result.Results[i].Series = unit.Series()
				result.Results[i].Life = params.Life(unit.Life().String())
				result.Results[i].Resolved = params.ResolvedMode(unit.Resolved())
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// CurrentModel returns the name and UUID for the current juju model.
func (u *UniterAPI) CurrentModel() (params.ModelResult, error) {
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
func (u *UniterAPI) ProviderType() (params.StringResult, error) {
	result := params.StringResult{}
	cfg, err := u.m.ModelConfig()
	if err == nil {
		result.Result = cfg.Type()
	}
	return result, err
}

// EnterScope ensures each unit has entered its scope in the relation,
// for all of the given relation/unit pairs. See also
// state.RelationUnit.EnterScope().
func (u *UniterAPI) EnterScope(args params.RelationUnits) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.RelationUnits)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.ErrorResults{}, err
	}
	one := func(relTag string, unitTag names.UnitTag, modelSubnets []string) error {
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
			logger.Debugf("ignoring %q EnterScope for %q - unit has invalid principal %q",
				unit.Name(), rel.String(), principalName)
			return nil
		}

		settings := map[string]interface{}{}
		ingressAddress, err := relUnit.IngressAddress()
		if err == nil {
			// private-address is historically a cloud local address for the machine.
			// Existing charms are built to ask for this attribute from relation
			// settings to find out what address to use to connect to the app
			// on the other side of a relation. For cross model scenarios, we'll
			// replace this with possibly a public address; we expect to fix more
			// charms than we break - breakage will not occur for correctly written
			// charms, since the semantics of this value dictates the use case described.
			// Any other use goes against the intended purpose of this value.
			settings["private-address"] = ingressAddress.Value
			// ingress-address is the preferred settings attribute name as it more accurately
			// reflects the purpose of the attribute value. We'll deprecate private-address.
			settings["ingress-address"] = ingressAddress.Value
		} else {
			logger.Warningf("cannot set ingress-address for unit %v in relation %v: %v", unitTag.Id(), relTag, err)
		}
		var egressSubnets []string
		egressNetworks := state.NewRelationEgressNetworks(u.st)
		rn, err := egressNetworks.Networks(rel.Tag().Id())
		if err == nil {
			// egress-subnets are a slice of cidrs from which traffic
			// on this side of the relation may originate.
			egressSubnets = rn.CIDRS()
		} else if errors.IsNotFound(err) {
			// No relation specific subnets, so maybe there's a model setting.
			if len(modelSubnets) > 0 {
				egressSubnets = modelSubnets
			} else if ingressAddress.Value != "" {
				// We default to the ingress address.
				cidrs := firewall.FormatAsCIDR([]string{ingressAddress.Value})
				egressSubnets = cidrs
			}
		} else {
			logger.Warningf("cannot set egress-subnets for unit %v in relation %v: %v", unitTag.Id(), relTag, err)
		}
		if len(egressSubnets) > 0 {
			settings["egress-subnets"] = strings.Join(egressSubnets, ",")
		}
		return relUnit.EnterScope(settings)
	}
	cfg, err := u.m.ModelConfig()
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	for i, arg := range args.RelationUnits {
		tag, err := names.ParseUnitTag(arg.Unit)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = one(arg.Relation, tag, cfg.EgressSubnets())
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
		}
	}
	return result, nil
}

// LeaveScope signals each unit has left its scope in the relation,
// for all of the given relation/unit pairs. See also
// state.RelationUnit.LeaveScope().
func (u *UniterAPI) LeaveScope(args params.RelationUnits) (params.ErrorResults, error) {
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
func (u *UniterAPI) ReadSettings(args params.RelationUnits) (params.SettingsResults, error) {
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
func (u *UniterAPI) ReadRemoteSettings(args params.RelationUnitPairs) (params.SettingsResults, error) {
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
func (u *UniterAPI) UpdateSettings(args params.RelationUnitsSettings) (params.ErrorResults, error) {
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
func (u *UniterAPI) WatchRelationUnits(args params.RelationUnits) (params.RelationUnitsWatchResults, error) {
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

// SetRelationStatus updates the status of the specified relations.
func (u *UniterAPI) SetRelationStatus(args params.RelationStatusArgs) (params.ErrorResults, error) {
	var statusResults params.ErrorResults

	// TODO(wallyworld) - the token should be passed to SetStatus() but the
	// interface method doesn't allow for that yet.
	checker := u.st.LeadershipChecker()
	token := checker.LeadershipCheck(u.unit.ApplicationName(), u.unit.Name())
	if err := token.Check(nil); err != nil {
		return statusResults, err
	}

	changeOne := func(arg params.RelationStatusArg) error {
		rel, err := u.st.Relation(arg.RelationId)
		if errors.IsNotFound(err) {
			return common.ErrPerm
		} else if err != nil {
			return errors.Trace(err)
		}
		_, err = rel.Unit(u.unit)
		if errors.IsNotFound(err) {
			return common.ErrPerm
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
		results[i].Error = common.ServerError(err)
	}
	statusResults.Results = results
	return statusResults, nil
}

// WatchUnitAddresses returns a NotifyWatcher for observing changes
// to each unit's addresses.
func (u *UniterAPI) WatchUnitAddresses(args params.Entities) (params.NotifyWatchResults, error) {
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

func (u *UniterAPI) getUnit(tag names.UnitTag) (*state.Unit, error) {
	return u.st.Unit(tag.Id())
}

func (u *UniterAPI) getApplication(tag names.ApplicationTag) (*state.Application, error) {
	return u.st.Application(tag.Id())
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
		// unit's application is not part of the requested
		// relation. That's why it's appropriate to return ErrPerm
		// here.
		return nothing, common.ErrPerm
	}
	return result, nil
}

func (u *UniterAPI) getRelationAndUnit(canAccess common.AuthFunc, relTag string, unitTag names.UnitTag) (*state.Relation, *state.Unit, error) {
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

func (u *UniterAPI) prepareRelationResult(rel *state.Relation, unit *state.Unit) (params.RelationResult, error) {
	nothing := params.RelationResult{}
	ep, err := rel.Endpoint(unit.ApplicationName())
	if err != nil {
		// An error here means the unit's application is not part of the
		// relation.
		return nothing, err
	}
	var otherAppName string
	otherEndpoints, err := rel.RelatedEndpoints(unit.ApplicationName())
	if err != nil {
		return nothing, err
	}
	for _, otherEp := range otherEndpoints {
		otherAppName = otherEp.ApplicationName
	}
	return params.RelationResult{
		Id:        rel.Id(),
		Key:       rel.String(),
		Life:      params.Life(rel.Life().String()),
		Suspended: rel.Suspended(),
		Endpoint: multiwatcher.Endpoint{
			ApplicationName: ep.ApplicationName,
			Relation:        multiwatcher.NewCharmRelation(ep.Relation),
		},
		OtherApplication: otherAppName,
	}, nil
}

func (u *UniterAPI) getOneRelation(canAccess common.AuthFunc, relTag, unitTag string) (params.RelationResult, error) {
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

func (u *UniterAPI) destroySubordinates(principal *state.Unit) error {
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

func (u *UniterAPI) watchOneUnitConfigSettings(tag names.UnitTag) (string, error) {
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

func (u *UniterAPI) watchOneUnitAddresses(tag names.UnitTag) (string, error) {
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

func (u *UniterAPI) watchOneRelationUnit(relUnit *state.RelationUnit) (params.RelationUnitsWatchResult, error) {
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

func (u *UniterAPI) checkRemoteUnit(relUnit *state.RelationUnit, remoteUnitTag string) (string, error) {
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
	remoteApplicationName, err := names.UnitApplication(remoteUnitName)
	if err != nil {
		return "", common.ErrPerm
	}
	rel := relUnit.Relation()
	_, err = rel.RelatedEndpoints(remoteApplicationName)
	if err != nil {
		return "", common.ErrPerm
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
		return "", watcher.EnsureErr(w)
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
		st.LeadershipChecker().LeadershipCheck,
		writeSettings,
	)
}

// AddMetricBatches adds the metrics for the specified unit.
func (u *UniterAPI) AddMetricBatches(args params.MetricBatchParams) (params.ErrorResults, error) {
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

// V4 specific methods.

//  specific methods - the new SLALevel, NetworkInfo and
// WatchUnitRelations methods.

// SLALevel returns the model's SLA level.
func (u *UniterAPI) SLALevel() (params.StringResult, error) {
	result := params.StringResult{}
	sla, err := u.st.SLALevel()
	if err == nil {
		result.Result = sla
	}
	return result, err
}

// NetworkInfo returns network interfaces/addresses for specified bindings.
func (u *UniterAPI) NetworkInfo(args params.NetworkInfoParams) (params.NetworkInfoResults, error) {
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.NetworkInfoResults{}, err
	}

	unitTag, err := names.ParseUnitTag(args.Unit)
	if err != nil {
		return params.NetworkInfoResults{}, err
	}

	if !canAccess(unitTag) {
		return params.NetworkInfoResults{}, common.ErrPerm
	}

	unit, err := u.getUnit(unitTag)
	if err != nil {
		return params.NetworkInfoResults{}, err
	}

	machineID, err := unit.AssignedMachineId()
	if err != nil {
		return params.NetworkInfoResults{}, err
	}

	machine, err := u.st.Machine(machineID)
	if err != nil {
		return params.NetworkInfoResults{}, err
	}

	result := params.NetworkInfoResults{
		Results: make(map[string]params.NetworkInfoResult),
	}

	spaces := set.NewStrings()
	bindingsToSpace := make(map[string]string)
	bindingsToEgressSubnets := make(map[string][]string)
	bindingsToIngressAddress := make(map[string]string)

	model, err := u.st.Model()
	if err != nil {
		return params.NetworkInfoResults{}, err
	}
	modelCfg, err := model.ModelConfig()
	if err != nil {
		return params.NetworkInfoResults{}, err
	}
	for _, binding := range args.Bindings {
		if boundSpace, err := unit.GetSpaceForBinding(binding); err != nil {
			result.Results[binding] = params.NetworkInfoResult{Error: common.ServerError(err)}
		} else {
			spaces.Add(boundSpace)
			bindingsToSpace[binding] = boundSpace
		}
		bindingsToEgressSubnets[binding] = modelCfg.EgressSubnets()
	}

	if args.RelationId != nil {
		// We're in a relation context.
		rel, err := u.st.Relation(*args.RelationId)
		if err != nil {
			return params.NetworkInfoResults{}, err
		}
		endpoint, err := rel.Endpoint(unit.ApplicationName())
		if err != nil {
			return params.NetworkInfoResults{}, err
		}

		// Look for any relation specific egress subnets.
		relEgress := state.NewRelationEgressNetworks(u.st)
		egressSubnets, err := relEgress.Networks(rel.Tag().Id())
		if err != nil && !errors.IsNotFound(err) {
			return params.NetworkInfoResults{}, err
		} else if err == nil {
			cidrs := egressSubnets.CIDRS()
			if len(cidrs) > 0 {
				bindingsToEgressSubnets[endpoint.Name] = cidrs
			}
		}

		boundSpace, err := unit.GetSpaceForBinding(endpoint.Name)
		if err != nil && !errors.IsNotValid(err) {
			return params.NetworkInfoResults{}, err
		}
		// If the endpoint for this relation is not bound to a space, or
		// is bound to the default space, we need to look up the ingress
		// address info which is aware of cross model relations.
		if boundSpace == environs.DefaultSpaceName || err != nil {
			ru, err := u.getRelationUnit(canAccess, rel.Tag().String(), unitTag)
			if err != nil {
				return params.NetworkInfoResults{}, err
			}
			address, err := ru.IngressAddress()
			if err != nil {
				return params.NetworkInfoResults{}, err
			}
			bindingsToIngressAddress[endpoint.Name] = address.Value
		}
	}

	networkInfos := machine.GetNetworkInfoForSpaces(spaces)
	for binding, space := range bindingsToSpace {
		// The binding address information based on link layer devices.
		info := networkingcommon.MachineNetworkInfoResultToNetworkInfoResult(networkInfos[space])

		// Set egress and ingress address information.
		info.EgressSubnets = bindingsToEgressSubnets[binding]
		// If there is no ingress address explicitly defined for a given binding,
		// set the ingress addresses to the binding addresses.
		ingressAddress, ok := bindingsToIngressAddress[binding]
		if !ok {
			for _, nwInfo := range info.Info {
				for _, addr := range nwInfo.Addresses {
					info.IngressAddresses = append(info.IngressAddresses, addr.Address)
				}
			}
		}
		if len(info.IngressAddresses) == 0 && ingressAddress != "" {
			info.IngressAddresses = []string{ingressAddress}
		}
		result.Results[binding] = info
	}

	return result, nil
}

// WatchUnitRelations returns a StringsWatcher, for each given
// unit, that notifies of changes to the lifecycles of relations
// relevant to that unit. For principal units, this will be all of the
// relations for the application. For subordinate units, only
// relations with the principal unit's application will be monitored.
func (u *UniterAPI) WatchUnitRelations(args params.Entities) (params.StringsWatchResults, error) {
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
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
		if canAccess(tag) {
			result.Results[i], err = u.watchOneUnitRelations(tag)
		}
		result.Results[i].Error = common.ServerError(err)
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
		watch, err = newSubordinateRelationsWatcher(u.st, app, principalApp.Name())
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
	return nothing, watcher.EnsureErr(watch)
}

// NetworkConfig returns information about all given relation/unit pairs,
// including their id, key and the local endpoint.
// It's not included in APIv5
// TODO(wpk): NetworkConfig API is obsoleted by Uniter.NetworkInfo
func (u *UniterAPIV4) NetworkConfig(args params.UnitsNetworkConfig) (params.UnitNetworkConfigResults, error) {
	result := params.UnitNetworkConfigResults{
		Results: make([]params.UnitNetworkConfigResult, len(args.Args)),
	}

	canAccess, err := u.accessUnit()
	if err != nil {
		return params.UnitNetworkConfigResults{}, err
	}

	for i, arg := range args.Args {
		netConfig, err := u.getOneNetworkConfig(canAccess, arg.UnitTag, arg.BindingName)
		if err == nil {
			result.Results[i].Config = netConfig
		} else {
			result.Results[i].Error = common.ServerError(err)
		}
	}
	return result, nil
}

func (u *UniterAPIV4) getOneNetworkConfig(canAccess common.AuthFunc, unitTagArg, bindingName string) ([]params.NetworkConfig, error) {
	unitTag, err := names.ParseUnitTag(unitTagArg)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if bindingName == "" {
		return nil, errors.Errorf("binding name cannot be empty")
	}

	if !canAccess(unitTag) {
		return nil, common.ErrPerm
	}

	unit, err := u.getUnit(unitTag)
	if err != nil {
		return nil, errors.Trace(err)
	}

	application, err := unit.Application()
	if err != nil {
		return nil, errors.Trace(err)
	}

	bindings, err := application.EndpointBindings()
	if err != nil {
		return nil, errors.Trace(err)
	}
	boundSpace, known := bindings[bindingName]
	if !known {
		return nil, errors.Errorf("binding name %q not defined by the unit's charm", bindingName)
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
	if boundSpace == "" {
		logger.Debugf(
			"endpoint %q not explicitly bound to a space, using preferred private address for machine %q",
			bindingName, machineID,
		)

		privateAddress, err := machine.PrivateAddress()
		if err != nil {
			return nil, errors.Annotatef(err, "getting machine %q preferred private address", machineID)
		}

		results = append(results, params.NetworkConfig{
			Address: privateAddress.Value,
		})
		return results, nil
	} else {
		logger.Debugf("endpoint %q is explicitly bound to space %q", bindingName, boundSpace)
	}

	// TODO(dimitern): Use NetworkInterfaces() instead later, this is just for
	// the PoC to enable minimal network-get implementation returning just the
	// primary address.
	//
	// LKK Card: https://canonical.leankit.com/Boards/View/101652562/119258804
	addresses, err := machine.AllAddresses()
	if err != nil {
		return nil, errors.Annotate(err, "cannot get devices addresses")
	}
	logger.Debugf(
		"getting network config for machine %q with addresses %+v, hosting unit %q of application %q, with bindings %+v",
		machineID, addresses, unit.Name(), application.Name(), bindings,
	)

	for _, addr := range addresses {
		subnet, err := addr.Subnet()
		if errors.IsNotFound(err) {
			logger.Debugf("skipping %s: not linked to a known subnet (%v)", addr, err)
			continue
		} else if err != nil {
			return nil, errors.Annotatef(err, "cannot get subnet for address %q", addr)
		}

		if space := subnet.SpaceName(); space != boundSpace {
			logger.Debugf("skipping %s: want bound to space %q, got space %q", addr, boundSpace, space)
			continue
		}
		logger.Debugf("endpoint %q bound to space %q has address %q", bindingName, boundSpace, addr)

		// TODO(dimitern): Fill in the rest later (see linked LKK card above).
		results = append(results, params.NetworkConfig{
			Address: addr.Value(),
		})
	}

	return results, nil
}

func relationResultsToV5(v6Results params.RelationResults) params.RelationResultsV5 {
	results := make([]params.RelationResultV5, len(v6Results.Results))
	for i, v6Result := range v6Results.Results {
		results[i].Error = v6Result.Error
		results[i].Life = v6Result.Life
		results[i].Id = v6Result.Id
		results[i].Key = v6Result.Key
		results[i].Endpoint = v6Result.Endpoint
	}
	return params.RelationResultsV5{Results: results}
}

// Relation returns information about all given relation/unit pairs,
// including their id, key and the local endpoint (without other
// application name).
func (u *UniterAPIV5) Relation(args params.RelationUnits) (params.RelationResultsV5, error) {
	v6Results, err := u.UniterAPI.Relation(args)
	if err != nil {
		return params.RelationResultsV5{}, errors.Trace(err)
	}
	return relationResultsToV5(v6Results), nil
}

// RelationById returns information about all given relations,
// specified by their ids, including their key and the local
// endpoint (without other application name).
func (u *UniterAPIV5) RelationById(args params.RelationIds) (params.RelationResultsV5, error) {
	v6Results, err := u.UniterAPI.RelationById(args)
	if err != nil {
		return params.RelationResultsV5{}, errors.Trace(err)
	}
	return relationResultsToV5(v6Results), nil
}

// WatchApplicationRelations returns a StringsWatcher, for each given
// application, that notifies of changes to the lifecycles of
// relations involving that application. This method is obsolete -
// it's been replaced by WatchUnitRelations in V5 of the uniter API.
func (u *UniterAPIV4) WatchApplicationRelations(args params.Entities) (params.StringsWatchResults, error) {
	result := params.StringsWatchResults{
		Results: make([]params.StringsWatchResult, len(args.Entities)),
	}
	canAccess, err := u.accessApplication()
	if err != nil {
		return params.StringsWatchResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseApplicationTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
		if canAccess(tag) {
			result.Results[i], err = u.watchOneApplicationRelations(tag)
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

func (u *UniterAPIV4) watchOneApplicationRelations(tag names.ApplicationTag) (params.StringsWatchResult, error) {
	nothing := params.StringsWatchResult{}
	application, err := u.getApplication(tag)
	if err != nil {
		return nothing, err
	}
	watch := application.WatchRelations()
	// Consume the initial event and forward it to the result.
	if changes, ok := <-watch.Changes(); ok {
		return params.StringsWatchResult{
			StringsWatcherId: u.resources.Register(watch),
			Changes:          changes,
		}, nil
	}
	return nothing, watcher.EnsureErr(watch)
}

// Mask the new methods from the V4 API. The API reflection code in
// rpc/rpcreflect/type.go:newMethod skips 2-argument methods, so this
// removes the method as far as the RPC machinery is concerned.

// SLALevel isn't on the V4 API.
func (u *UniterAPIV4) SLALevel(_, _ struct{}) {}

// NetworkInfo isn't on the V4 API.
func (u *UniterAPIV4) NetworkInfo(_, _ struct{}) {}

// WatchUnitRelations isn't on the V4 API.
func (u *UniterAPIV4) WatchUnitRelations(_, _ struct{}) {}
