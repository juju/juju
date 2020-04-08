// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package uniter implements the API interface used by the uniter worker.
package uniter

import (
	"fmt"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/cloudspec"
	"github.com/juju/juju/apiserver/facade"
	leadershipapiserver "github.com/juju/juju/apiserver/facades/agent/leadership"
	"github.com/juju/juju/apiserver/facades/agent/meterstatus"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/caas"
	k8sspecs "github.com/juju/juju/caas/kubernetes/provider/specs"
	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/life"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
	"github.com/juju/juju/state/watcher"
)

var logger = loggo.GetLogger("juju.apiserver.uniter")

// UniterAPI implements the latest version (v15) of the Uniter API, which adds
// the State, CommitHookChanges calls and changes WatchActionNotifications to
// notify on action changes.
type UniterAPI struct {
	*common.LifeGetter
	*StatusAPI
	*common.DeadEnsurer
	*common.AgentEntityWatcher
	*common.APIAddresser
	*common.ModelWatcher
	*common.RebootRequester
	*common.UpgradeSeriesAPI
	*common.UnitStateAPI
	*leadershipapiserver.LeadershipSettingsAccessor
	meterstatus.MeterStatus
	m                   *state.Model
	st                  *state.State
	clock               clock.Clock
	cancel              <-chan struct{}
	auth                facade.Authorizer
	resources           facade.Resources
	leadershipChecker   leadership.Checker
	accessUnit          common.GetAuthFunc
	accessApplication   common.GetAuthFunc
	accessMachine       common.GetAuthFunc
	containerBrokerFunc caas.NewContainerBrokerFunc
	*StorageAPI

	// cacheModel is used to access data from the cache in lieu of going
	// to the database.
	// TODO (manadart 2019-06-20): Use cache to watch and retrieve model config.
	cacheModel *cache.Model

	// A cloud spec can only be accessed for the model of the unit or
	// application that is authorised for this API facade.
	// We do not need to use an AuthFunc, because we do not need to pass a tag.
	accessCloudSpec func() (func() bool, error)
	cloudSpec       cloudspec.CloudSpecAPI
}

// UniterAPIV14 implements version (v14) of the Uniter API,
// which adds GetPodSpec, SetState and State.
type UniterAPIV14 struct {
	UniterAPI
}

// UniterAPIV13 implements version (v13) of the Uniter API,
// which adds UpdateNetworkInfo.
type UniterAPIV13 struct {
	UniterAPIV14
}

// UniterAPIV12 implements version (v12) of the Uniter API,
// Removes the embedded LXDProfileAPI, which in turn removes the following;
// RemoveUpgradeCharmProfileData, WatchUnitLXDProfileUpgradeNotifications
// and WatchLXDProfileUpgradeNotifications
type UniterAPIV12 struct {
	UniterAPIV13
}

// UniterAPIV11 implements version (v11) of the Uniter API, which adds
// CloudAPIVersion.
type UniterAPIV11 struct {
	*LXDProfileAPI
	UniterAPIV12
}

// UniterAPIV10 adds WatchUnitLXDProfileUpgradeNotifications and
type UniterAPIV10 struct {
	// LXDProfileAPI is removed from a UniterAPI embedded struct to UniterAPIV10
	// embedded struct removing it completely from future API versions.
	*LXDProfileAPI
	UniterAPIV11
}

// UniterAPIV9 adds WatchConfigSettingsHash, WatchTrustConfigSettingsHash,
// WatchUnitAddressesHash and LXDProfileAPI, which includes
// WatchLXDProfileUpgradeNotifications and RemoveUpgradeCharmProfileData
type UniterAPIV9 struct {
	// LXDProfileAPI is removed from a UniterAPI embedded struct to UniterAPIV9
	// embedded struct removing it completely from future API versions.
	*LXDProfileAPI
	UniterAPIV10
}

// UniterAPIV8 adds SetContainerSpec, GoalStates, CloudSpec,
// WatchTrustConfigSettings, WatchActionNotifications,
// UpgradeSeriesStatus, SetUpgradeSeriesStatus.
type UniterAPIV8 struct {
	UniterAPIV9
}

// UniterAPIV7 adds CMR support to NetworkInfo.
type UniterAPIV7 struct {
	UniterAPIV8
}

// UniterAPIV6 adds NetworkInfo as a preferred method to calling NetworkConfig.
type UniterAPIV6 struct {
	UniterAPIV7
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
func NewUniterAPI(context facade.Context) (*UniterAPI, error) {
	authorizer := context.Auth()
	if !authorizer.AuthUnitAgent() && !authorizer.AuthApplicationAgent() {
		return nil, common.ErrPerm
	}
	st := context.State()
	aClock := context.StatePool().Clock()
	resources := context.Resources()
	leadershipChecker, err := context.LeadershipChecker()
	if err != nil {
		return nil, errors.Trace(err)
	}

	accessUnit := unitAccessor(authorizer, st)
	accessApplication := applicationAccessor(authorizer, st)
	accessMachine := machineAccessor(authorizer, st)
	accessCloudSpec := cloudSpecAccessor(authorizer, st)

	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	storageAccessor, err := getStorageState(st)
	if err != nil {
		return nil, errors.Trace(err)
	}
	storageAPI, err := newStorageAPI(
		stateShim{st}, storageAccessor, resources, accessUnit)
	if err != nil {
		return nil, errors.Trace(err)
	}

	msAPI, err := meterstatus.NewMeterStatusAPI(st, resources, authorizer)
	if err != nil {
		return nil, errors.Annotate(err, "could not create meter status API handler")
	}
	accessUnitOrApplication := common.AuthAny(accessUnit, accessApplication)

	cloudSpec := cloudspec.NewCloudSpec(resources,
		cloudspec.MakeCloudSpecGetterForModel(st),
		cloudspec.MakeCloudSpecWatcherForModel(st),
		cloudspec.MakeCloudSpecCredentialWatcherForModel(st),
		cloudspec.MakeCloudSpecCredentialContentWatcherForModel(st),
		common.AuthFuncForTag(m.ModelTag()),
	)

	cacheModel, err := context.Controller().Model(st.ModelUUID())
	if err != nil {
		return nil, err
	}

	return &UniterAPI{
		LifeGetter:                 common.NewLifeGetter(st, accessUnitOrApplication),
		DeadEnsurer:                common.NewDeadEnsurer(st, accessUnit),
		AgentEntityWatcher:         common.NewAgentEntityWatcher(st, resources, accessUnitOrApplication),
		APIAddresser:               common.NewAPIAddresser(st, resources),
		ModelWatcher:               common.NewModelWatcher(m, resources, authorizer),
		RebootRequester:            common.NewRebootRequester(st, accessMachine),
		UpgradeSeriesAPI:           common.NewExternalUpgradeSeriesAPI(st, resources, authorizer, accessMachine, accessUnit, logger),
		UnitStateAPI:               common.NewExternalUnitStateAPI(st, resources, authorizer, accessUnit, logger),
		LeadershipSettingsAccessor: leadershipSettingsAccessorFactory(st, leadershipChecker, resources, authorizer),
		MeterStatus:                msAPI,
		// TODO(fwereade): so *every* unit should be allowed to get/set its
		// own status *and* its application's? This is not a pleasing arrangement.
		StatusAPI: NewStatusAPI(st, accessUnitOrApplication, leadershipChecker),

		m:                 m,
		st:                st,
		clock:             aClock,
		cancel:            context.Cancel(),
		cacheModel:        cacheModel,
		auth:              authorizer,
		resources:         resources,
		leadershipChecker: leadershipChecker,
		accessUnit:        accessUnit,
		accessApplication: accessApplication,
		accessMachine:     accessMachine,
		accessCloudSpec:   accessCloudSpec,
		cloudSpec:         cloudSpec,
		StorageAPI:        storageAPI,
	}, nil
}

// NewUniterAPIV14 creates an instance of the V14 uniter API.
func NewUniterAPIV14(context facade.Context) (*UniterAPIV14, error) {
	uniterAPI, err := NewUniterAPI(context)
	if err != nil {
		return nil, err
	}
	return &UniterAPIV14{
		UniterAPI: *uniterAPI,
	}, nil
}

// NewUniterAPIV13 creates an instance of the V13 uniter API.
func NewUniterAPIV13(context facade.Context) (*UniterAPIV13, error) {
	uniterAPI, err := NewUniterAPIV14(context)
	if err != nil {
		return nil, err
	}
	return &UniterAPIV13{
		UniterAPIV14: *uniterAPI,
	}, nil
}

// NewUniterAPIV12 creates an instance of the V12 uniter API.
func NewUniterAPIV12(context facade.Context) (*UniterAPIV12, error) {
	uniterAPI, err := NewUniterAPIV13(context)
	if err != nil {
		return nil, err
	}
	return &UniterAPIV12{
		UniterAPIV13: *uniterAPI,
	}, nil
}

// NewUniterAPIV11 creates an instance of the V11 uniter API.
func NewUniterAPIV11(context facade.Context) (*UniterAPIV11, error) {
	uniterAPI, err := NewUniterAPIV12(context)
	if err != nil {
		return nil, err
	}
	authorizer := context.Auth()
	st := context.State()
	resources := context.Resources()
	accessUnit := unitAccessor(authorizer, st)
	return &UniterAPIV11{
		LXDProfileAPI: NewExternalLXDProfileAPI(st, resources, authorizer, accessUnit, logger),
		UniterAPIV12:  *uniterAPI,
	}, nil
}

// NewUniterAPIV10 creates an instance of the V10 uniter API.
func NewUniterAPIV10(context facade.Context) (*UniterAPIV10, error) {
	uniterAPI, err := NewUniterAPIV11(context)
	if err != nil {
		return nil, err
	}

	return &UniterAPIV10{
		LXDProfileAPI: uniterAPI.LXDProfileAPI,
		UniterAPIV11:  *uniterAPI,
	}, nil
}

// NewUniterAPIV9 creates an instance of the V9 uniter API.
func NewUniterAPIV9(context facade.Context) (*UniterAPIV9, error) {
	uniterAPI, err := NewUniterAPIV10(context)
	if err != nil {
		return nil, err
	}
	return &UniterAPIV9{
		LXDProfileAPI: uniterAPI.LXDProfileAPI,
		UniterAPIV10:  *uniterAPI,
	}, nil
}

// NewUniterAPIV8 creates an instance of the V8 uniter API.
func NewUniterAPIV8(context facade.Context) (*UniterAPIV8, error) {
	uniterAPI, err := NewUniterAPIV9(context)
	if err != nil {
		return nil, err
	}
	return &UniterAPIV8{
		UniterAPIV9: *uniterAPI,
	}, nil
}

// NewUniterAPIV7 creates an instance of the V7 uniter API.
func NewUniterAPIV7(context facade.Context) (*UniterAPIV7, error) {
	uniterAPI, err := NewUniterAPIV8(context)
	if err != nil {
		return nil, err
	}
	return &UniterAPIV7{
		UniterAPIV8: *uniterAPI,
	}, nil
}

// NewUniterAPIV6 creates an instance of the V6 uniter API.
func NewUniterAPIV6(context facade.Context) (*UniterAPIV6, error) {
	uniterAPI, err := NewUniterAPIV7(context)
	if err != nil {
		return nil, err
	}
	return &UniterAPIV6{
		UniterAPIV7: *uniterAPI,
	}, nil
}

// NewUniterAPIV5 creates an instance of the V5 uniter API.
func NewUniterAPIV5(context facade.Context) (*UniterAPIV5, error) {
	uniterAPI, err := NewUniterAPIV6(context)
	if err != nil {
		return nil, err
	}
	return &UniterAPIV5{
		UniterAPIV6: *uniterAPI,
	}, nil
}

// NewUniterAPIV4 creates an instance of the V4 uniter API.
func NewUniterAPIV4(context facade.Context) (*UniterAPIV4, error) {
	uniterAPI, err := NewUniterAPIV5(context)
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
		portRanges := make([]corenetwork.PortRange, 0, len(portRangesToUnits))
		for portRange := range portRangesToUnits {
			portRanges = append(portRanges, portRange)
		}
		corenetwork.SortPortRanges(portRanges)
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
				var address corenetwork.SpaceAddress
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
				var address corenetwork.SpaceAddress
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
// be returned if a unit is dead, or the charm URL is not known.
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
				if err == nil {
					// Wait for the change to propagate to the cache controller.
					err = u.waitForControllerCharmURL(unit.Name(), curl.String())
				}
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

func (u *UniterAPI) waitForControllerCharmURL(unit, curl string) error {
	// One minute is excessively long, but the cache may need to refresh.
	// In any normal operation, this should be sub-second, although if no changes
	// were happening recently, it could be theoretically up to five seconds.
	timeout := u.clock.After(time.Minute)
	cancel := make(chan struct{})
	done := u.cacheModel.WaitForUnit(unit, func(u *cache.Unit) bool {
		return u.CharmURL() == curl
	}, cancel)

	select {
	case <-done:
		return nil
	case <-u.cancel:
		// The API server is stopping, so don't wait any longer.
		close(cancel)
		return errors.Timeoutf("apiserver stopping, unit change %s.CharmURL to %q", unit, curl)
	case <-timeout:
		close(cancel)
		return errors.Timeoutf("unit change %s.CharmURL to %q", unit, curl)
	}
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
		if !canAccess(tag) {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}

		unit, err := u.getUnit(tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}

		openPortRange := []corenetwork.PortRange{{
			FromPort: entity.FromPort,
			ToPort:   entity.ToPort,
			Protocol: entity.Protocol,
		}}

		err = unit.OpenClosePortsOnSubnet("", openPortRange, nil)
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
		if !canAccess(tag) {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}

		unit, err := u.getUnit(tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}

		closePortRange := []corenetwork.PortRange{{
			FromPort: entity.FromPort,
			ToPort:   entity.ToPort,
			Protocol: entity.Protocol,
		}}

		err = unit.OpenClosePortsOnSubnet("", nil, closePortRange)
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// WatchConfigSettings returns a NotifyWatcher for observing changes
// to each unit's application configuration settings. See also
// state/watcher.go:Unit.WatchConfigSettings().
func (u *UniterAPIV8) WatchConfigSettings(args params.Entities) (params.NotifyWatchResults, error) {
	watcherFn := func(u *state.Unit) (state.NotifyWatcher, error) {
		return u.WatchConfigSettings()
	}
	result, err := u.WatchSettings(args, watcherFn)
	if err != nil {
		return params.NotifyWatchResults{}, errors.Trace(err)
	}

	return result, nil
}

func (u *UniterAPIV8) WatchTrustConfigSettings(args params.Entities) (params.NotifyWatchResults, error) {
	watcherFn := func(u *state.Unit) (state.NotifyWatcher, error) {
		return u.WatchApplicationConfigSettings()
	}
	result, err := u.WatchSettings(args, watcherFn)
	if err != nil {
		return params.NotifyWatchResults{}, errors.Trace(err)
	}

	return result, nil
}

func (u *UniterAPIV8) WatchSettings(args params.Entities, configWatcherFn func(u *state.Unit) (state.NotifyWatcher, error)) (params.NotifyWatchResults, error) {
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
			watcherId, err = u.watchOneUnitConfigSettings(tag, configWatcherFn)
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

// WatchActionNotifications preserves previous functionality of the ActionNotifications watcher
// to only trigger once on creation of a pending Action.
func (u *UniterAPIV14) WatchActionNotifications(args params.Entities) (params.StringsWatchResults, error) {
	tagToActionReceiver := common.TagToActionReceiverFn(u.st.FindEntity)
	watchOne := common.WatchPendingActionsForReceiver(tagToActionReceiver, u.resources.Register)
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
			var unit cache.Unit
			unit, err = u.getCacheUnit(tag)
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

// ActionStatus returns the status of Actions by Tags passed in.
func (u *UniterAPI) ActionStatus(args params.Entities) (params.StringResults, error) {
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
			results.Results[k].Error = common.ServerError(err)
			continue
		}
		results.Results[k].Result = string(action.Status())
	}

	return results, nil
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

// LogActionsMessages records the log messages against the specified actions.
func (u *UniterAPI) LogActionsMessages(args params.ActionMessageParams) (params.ErrorResults, error) {
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
		result.Results[i].Error = common.ServerError(
			oneActionMessage(actionMessage.Tag, actionMessage.Value))
	}
	return result, nil
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
				result.Results[i].Life = life.Value(unit.Life().String())
				result.Results[i].Resolved = params.ResolvedMode(unit.Resolved())

				var err1 error
				result.Results[i].ProviderID, err1 = u.getProviderID(unit)
				if err1 != nil && !errors.IsNotFound(err1) {
					// initially, it returns not found error, so just ignore it.
					err = err1
				}
			}
		}
		result.Results[i].Error = common.ServerError(err)
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
func (u *UniterAPI) CurrentModel() (params.ModelResult, error) {
	result := params.ModelResult{}
	model, err := u.st.Model()
	if err == nil {
		result.Name = model.Name()
		result.UUID = model.UUID()
		result.Type = string(model.Type())
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

		netInfo, err := NewNetworkInfo(u.st, unitTag)
		if err != nil {
			return err
		}

		settings := map[string]interface{}{}
		_, ingressAddresses, egressSubnets, err := netInfo.NetworksForRelation(relUnit.Endpoint().Name, rel, false)
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
			logger.Warningf("cannot set ingress/egress addresses for unit %v in relation %v: %v",
				unitTag.Id(), relTag, err)
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
	canAccessUnit, err := u.accessUnit()
	if err != nil {
		return params.SettingsResults{}, errors.Trace(err)
	}
	canAccessApp, err := u.accessApplication()
	if err != nil {
		return params.SettingsResults{}, errors.Trace(err)
	}

	readOneSettings := func(arg params.RelationUnit) (params.Settings, error) {
		tag, err := names.ParseTag(arg.Unit)
		if err != nil {
			return nil, common.ErrPerm
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
			settings = node.Map()

		case names.ApplicationTag:
			var relation *state.Relation
			relation, err = u.getRelation(arg.Relation)
			if err != nil {
				return nil, errors.Trace(err)
			}
			endpoints := relation.Endpoints()
			isPeerRelation := len(endpoints) == 1 && endpoints[0].Role == charm.RolePeer
			token := u.leadershipChecker.LeadershipCheck(tag.Id(), u.auth.GetAuthTag().Id())
			canAccess := func(tag names.Tag) bool {
				if !canAccessApp(tag) {
					return false
				}
				if isPeerRelation {
					return true
				}
				// For provider-requirer relations only allow the
				// leader unit to read the application settings.
				return token.Check(0, nil) == nil
			}
			settings, err = u.getRelationAppSettings(canAccess, arg.Relation, tag)

		default:
			return nil, common.ErrPerm
		}

		if err != nil {
			return nil, errors.Trace(err)
		}
		return convertRelationSettings(settings)
	}

	for i, arg := range args.RelationUnits {
		settings, err := readOneSettings(arg)
		result.Results[i].Error = common.ServerError(err)
		result.Results[i].Settings = settings
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

	readOneSettings := func(arg params.RelationUnitPair) (params.Settings, error) {
		unit, err := names.ParseUnitTag(arg.LocalUnit)
		if err != nil {
			return nil, common.ErrPerm
		}
		relUnit, err := u.getRelationUnit(canAccess, arg.Relation, unit)
		if err != nil {
			return nil, errors.Trace(err)
		}
		remoteTag, err := names.ParseTag(arg.RemoteUnit)
		if err != nil {
			return nil, common.ErrPerm
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
			return nil, common.ErrPerm
		}
		if err != nil {
			return nil, errors.Trace(err)
		}
		return convertRelationSettings(settings)
	}

	for i, arg := range args.RelationUnitPairs {
		settings, err := readOneSettings(arg)
		result.Results[i].Error = common.ServerError(err)
		result.Results[i].Settings = settings
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
		updateOp, err := u.updateUnitAndApplicationSettingsOp(arg, canAccess)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}

		if err = u.st.ApplyOperation(updateOp); err != nil {
			if leadership.IsNotLeaderError(err) {
				err = common.ErrPerm
			}

			result.Results[i].Error = common.ServerError(err)
		}
	}
	return result, nil
}

func (u *UniterAPI) updateUnitAndApplicationSettingsOp(arg params.RelationUnitSettings, canAccess common.AuthFunc) (state.ModelOperation, error) {
	unitTag, err := names.ParseUnitTag(arg.Unit)
	if err != nil {
		return nil, common.ErrPerm
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
		if errors.IsNotFound(err) {
			return nil, common.ErrPerm
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
		if err := token.Check(0, nil); err != nil {
			return errors.Trace(err)
		}

		rel, err := u.st.Relation(arg.RelationId)
		if errors.IsNotFound(err) {
			return common.ErrPerm
		} else if err != nil {
			return errors.Trace(err)
		}
		_, err = rel.Unit(unit)
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
func (u *UniterAPIV8) WatchUnitAddresses(args params.Entities) (params.NotifyWatchResults, error) {
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

func (u *UniterAPI) getCacheUnit(tag names.UnitTag) (cache.Unit, error) {
	unit, err := u.cacheModel.Unit(tag.Id())
	return unit, errors.Trace(err)
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
		return nothing, common.ErrPerm
	}
	return result, nil
}

func (u *UniterAPI) getRelation(relTag string) (*state.Relation, error) {
	tag, err := names.ParseRelationTag(relTag)
	if err != nil {
		return nil, common.ErrPerm
	}
	rel, err := u.st.KeyRelation(tag.Id())
	if errors.IsNotFound(err) {
		return nil, common.ErrPerm
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
		return nil, nil, common.ErrPerm
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
		return nothing, common.ErrPerm
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
		return nil, common.ErrPerm
	}
	rel, err := u.st.KeyRelation(tag.Id())
	if errors.IsNotFound(err) {
		return nil, common.ErrPerm
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	if !canAccess(appTag) {
		return nil, common.ErrPerm
	}

	settings, err := rel.ApplicationSettings(appTag.Id())
	if errors.IsNotFound(err) {
		return nil, common.ErrPerm
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
		return nil, common.ErrPerm
	}

	var isRelatedToLocalApp bool
	for _, ep := range relatedEPs {
		if appTag.Id() == ep.ApplicationName {
			isRelatedToLocalApp = true
			break
		}
	}

	if !isRelatedToLocalApp {
		return nil, common.ErrPerm
	}

	return rel.ApplicationSettings(appTag.Id())
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

func (u *UniterAPIV8) watchOneUnitConfigSettings(tag names.UnitTag, configWatcherFn func(u *state.Unit) (state.NotifyWatcher, error)) (string, error) {
	unit, err := u.getUnit(tag)
	if err != nil {
		return "", err
	}
	configWatcher, err := configWatcherFn(unit)
	if err != nil {
		return "", errors.Trace(err)
	}
	// Consume the initial event. Technically, API
	// calls to Watch 'transmit' the initial event
	// in the Watch response. But NotifyWatchers
	// have no state to transmit.
	if _, ok := <-configWatcher.Changes(); ok {
		return u.resources.Register(configWatcher), nil
	}
	return "", watcher.EnsureErr(configWatcher)
}

func (u *UniterAPIV8) watchOneUnitAddresses(tag names.UnitTag) (string, error) {
	unit, err := u.getUnit(tag)
	if err != nil {
		return "", err
	}
	var watch state.NotifyWatcher
	if unit.ShouldBeAssigned() {
		machineId, err := unit.AssignedMachineId()
		if err != nil {
			return "", err
		}
		machine, err := u.st.Machine(machineId)
		if err != nil {
			return "", err
		}
		watch = machine.WatchAddresses()
	} else {
		watch = unit.WatchContainerAddresses()
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
	return params.RelationUnitsWatchResult{}, watcher.EnsureErr(watch)
}

func (u *UniterAPI) checkRemoteUnit(relUnit *state.RelationUnit, remoteUnitTag names.UnitTag) (string, error) {
	// Make sure the unit is indeed remote.
	switch tag := u.auth.GetAuthTag().(type) {
	case names.UnitTag:
		if remoteUnitTag == tag {
			return "", common.ErrPerm
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
				return "", common.ErrPerm
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
		checker.LeadershipCheck,
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
				Key:    metric.Key,
				Value:  metric.Value,
				Time:   metric.Time,
				Labels: metric.Labels,
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

	netInfo, err := NewNetworkInfo(u.st, unitTag)
	if err != nil {
		return params.NetworkInfoResults{}, err
	}

	return netInfo.ProcessAPIRequest(args)
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
	boundSpace, known := bindings.Map()[bindingName]
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
	if boundSpace == corenetwork.AlphaSpaceId {
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

		if space := subnet.SpaceID(); space != boundSpace {
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

func networkInfoResultsToV6(v7Results params.NetworkInfoResults) params.NetworkInfoResultsV6 {
	results := make(map[string]params.NetworkInfoResultV6)
	for k, v6Result := range v7Results.Results {
		results[k] = params.NetworkInfoResultV6{Error: v6Result.Error, Info: v6Result.Info}
	}
	return params.NetworkInfoResultsV6{Results: results}
}

// NetworkInfo implements UniterAPIV6 version of NetworkInfo by constructing an API V6 compatible result.
func (u *UniterAPIV6) NetworkInfo(args params.NetworkInfoParams) (params.NetworkInfoResultsV6, error) {
	v6Results, err := u.UniterAPI.NetworkInfo(args)
	if err != nil {
		return params.NetworkInfoResultsV6{}, errors.Trace(err)
	}
	return networkInfoResultsToV6(v6Results), nil
}

// Mask the SetPodSpec method from the v7 API. The API reflection code
// in rpc/rpcreflect/type.go:newMethod skips 2-argument methods, so
// this removes the method as far as the RPC machinery is concerned.

// SetPodSpec isn't on the v7 API.
func (u *UniterAPIV7) SetPodSpec(_, _ struct{}) {}

// SetPodSpec sets the pod specs for a set of applications. This call is kept
// here for backwards compatibility with V14 clients. Clients that support V15+
// of the facade will use the CommitHookChanges API call instead.
func (u *UniterAPIV14) SetPodSpec(args params.SetPodSpecParams) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Specs)),
	}
	authTag := u.auth.GetAuthTag()
	canAccessApp := makeAppAuthChecker(authTag)

	for i, arg := range args.Specs {
		results.Results[i].Error = common.ServerError(
			// NOTE(achilleasa) The operator authenticates as the
			// application so we cannot extract the unit id for
			// leadership check purposes. To this end we pass
			// nil as the unit tag to bypass the leadership check.
			// Newer controllers will use the CommitHookChanges
			// call which does perform the leadership check.
			u.setPodSpec(arg.Tag, &arg.Value, nil, canAccessApp),
		)
	}
	return results, nil
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

func (u *UniterAPI) setPodSpec(appTag string, spec *string, unitTag names.Tag, canAccessApp common.AuthFunc) error {
	modelOp, err := u.setPodSpecOperation(appTag, spec, unitTag, canAccessApp)
	if err != nil {
		return err
	}
	return u.st.ApplyOperation(modelOp)
}

func (u *UniterAPI) setPodSpecOperation(appTag string, spec *string, unitTag names.Tag, canAccessApp common.AuthFunc) (state.ModelOperation, error) {
	parsedAppTag, err := names.ParseApplicationTag(appTag)
	if err != nil {
		return nil, err
	}
	if !canAccessApp(parsedAppTag) {
		return nil, common.ErrPerm
	}
	if spec != nil {
		if _, err := k8sspecs.ParsePodSpec(*spec); err != nil {
			return nil, errors.Annotate(err, "invalid pod spec")
		}
	}

	cm, err := u.m.CAASModel()
	if err != nil {
		return nil, err
	}

	// If this call is invoked by the CommitHookChanges call, the unit tag
	// is known and can be used for a leadership check. For calls to the
	// SetPodSpec API endpoint (older k8s deployments) the unit tag is
	// unknown since the uniter authenticates as an application. In the
	// latter case, the leadership check is skipped.
	var token leadership.Token
	if unitTag != nil {
		token = u.leadershipChecker.LeadershipCheck(parsedAppTag.Id(), unitTag.Id())
	}
	return cm.SetPodSpecOperation(token, parsedAppTag, spec), nil
}

func (u *UniterAPI) setRawK8sSpecOperation(appTag string, spec *string, unitTag names.Tag, canAccessApp common.AuthFunc) (state.ModelOperation, error) {
	controllerCfg, err := u.st.ControllerConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if !controllerCfg.Features().Contains(feature.RawK8sSpec) {
		return nil, errors.NewNotSupported(nil,
			fmt.Sprintf("feature flag %q is required for setting raw k8s spec", feature.RawK8sSpec),
		)
	}

	parsedAppTag, err := names.ParseApplicationTag(appTag)
	if err != nil {
		return nil, err
	}
	if !canAccessApp(parsedAppTag) {
		return nil, common.ErrPerm
	}
	if spec != nil {
		if _, err := k8sspecs.ParseRawK8sSpec(*spec); err != nil {
			return nil, errors.Annotate(err, "invalid raw k8s spec")
		}
	}

	cm, err := u.m.CAASModel()
	if err != nil {
		return nil, err
	}

	var token leadership.Token
	if unitTag != nil {
		token = u.leadershipChecker.LeadershipCheck(parsedAppTag.Id(), unitTag.Id())
	}
	return cm.SetRawK8sSpecOperation(token, parsedAppTag, spec), nil
}

// Mask the GetPodSpec method from the v13 API. The API reflection code
// in rpc/rpcreflect/type.go:newMethod skips 2-argument methods, so
// this removes the method as far as the RPC machinery is concerned.

// GetPodSpec isn't on the v13 API.
func (u *UniterAPIV13) GetPodSpec(_, _ struct{}) {}

// GetPodSpec gets the pod specs for a set of applications.
func (u *UniterAPI) GetPodSpec(args params.Entities) (params.StringResults, error) {
	return u.getContainerSpec(args, func(m caasSpecGetter) getSpecFunc {
		return m.PodSpec
	})
}

// Mask the GetRawK8sSpec method from the v14 API. The API reflection code
// in rpc/rpcreflect/type.go:newMethod skips 2-argument methods, so
// this removes the method as far as the RPC machinery is concerned.

// GetRawK8sSpec isn't on the v14 API.
func (u *UniterAPIV14) GetRawK8sSpec(_, _ struct{}) {}

// GetRawK8sSpec gets the raw k8s specs for a set of applications.
func (u *UniterAPI) GetRawK8sSpec(args params.Entities) (params.StringResults, error) {
	return u.getContainerSpec(args, func(m caasSpecGetter) getSpecFunc {
		return m.RawK8sSpec
	})
}

type caasSpecGetter interface {
	PodSpec(names.ApplicationTag) (string, error)
	RawK8sSpec(names.ApplicationTag) (string, error)
}

type getSpecFunc func(names.ApplicationTag) (string, error)
type getSpecFuncGetter func(caasSpecGetter) getSpecFunc

func (u *UniterAPI) getContainerSpec(args params.Entities, getSpec getSpecFuncGetter) (params.StringResults, error) {
	results := params.StringResults{
		Results: make([]params.StringResult, len(args.Entities)),
	}
	authTag := u.auth.GetAuthTag()
	canAccess := func(tag names.Tag) bool {
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

	for i, arg := range args.Entities {
		tag, err := names.ParseApplicationTag(arg.Tag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		if !canAccess(tag) {
			results.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		cm, err := u.m.CAASModel()
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		spec, err := getSpec(cm)(tag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		results.Results[i].Result = spec
	}
	return results, nil
}

// CloudSpec returns the cloud spec used by the model in which the
// authenticated unit or application resides.
// A check is made beforehand to ensure that the request is made by an entity
// that has been granted the appropriate trust.
func (u *UniterAPI) CloudSpec() (params.CloudSpecResult, error) {
	canAccess, err := u.accessCloudSpec()
	if err != nil {
		return params.CloudSpecResult{}, err
	}
	if !canAccess() {
		return params.CloudSpecResult{Error: common.ServerError(common.ErrPerm)}, nil
	}

	return u.cloudSpec.GetCloudSpec(u.m.Tag().(names.ModelTag)), nil
}

// GoalStates returns information of charm units and relations.
func (u *UniterAPI) GoalStates(args params.Entities) (params.GoalStateResults, error) {
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
		result.Results[i].Result, err = u.oneGoalState(unit)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
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
			} else if errors.IsNotFound(err) {
				logger.Debugf("application %q must be a remote application.", e.ApplicationName)
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
			logger.Debugf("unit %q is dead, ignore it.", unit.Name())
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
func (u *UniterAPI) WatchConfigSettingsHash(args params.Entities) (params.StringsWatchResults, error) {
	result := params.StringsWatchResults{
		Results: make([]params.StringsWatchResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.StringsWatchResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil || !canAccess(tag) {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}

		unit, err := u.getCacheUnit(tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}

		w, err := unit.WatchConfigSettings()
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}

		// Consume the initial event.
		changes, ok := <-w.Changes()
		if !ok {
			result.Results[i].Error = common.ServerError(watcher.EnsureErr(w))
			continue
		}

		result.Results[i].Changes = changes
		result.Results[i].StringsWatcherId = u.resources.Register(w)
	}

	return result, nil
}

// WatchTrustConfigSettingsHash returns a StringsWatcher that yields a
// hash of the application config values whenever they change. The
// uniter can use the hash to determine whether the actual values have
// changed since it last saw the config.
func (u *UniterAPI) WatchTrustConfigSettingsHash(args params.Entities) (params.StringsWatchResults, error) {
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
func (u *UniterAPI) WatchUnitAddressesHash(args params.Entities) (params.StringsWatchResults, error) {
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

// Mask WatchConfigSettingsHash from the v8 API. The API reflection
// code in rpc/rpcreflect/type.go:newMethod skips 2-argument methods,
// so this removes the method as far as the RPC machinery is
// concerned.

// WatchConfigSettingsHash isn't on the v8 API.
func (u *UniterAPIV8) WatchConfigSettingsHash(_, _ struct{}) {}

// WatchTrustConfigSettingsHash isn't on the v8 API.
func (u *UniterAPIV8) WatchTrustConfigSettingsHash(_, _ struct{}) {}

// WatchUnitAddressesHash isn't on the v8 API.
func (u *UniterAPIV8) WatchUnitAddressesHash(_, _ struct{}) {}

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
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
		watcherId := ""
		var changes []string
		if canAccess(tag) {
			watcherId, changes, err = u.watchOneUnitHashes(tag, getWatcher)
		}
		result.Results[i].StringsWatcherId = watcherId
		result.Results[i].Changes = changes
		result.Results[i].Error = common.ServerError(err)
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
	return "", nil, watcher.EnsureErr(w)
}

// CloudAPIVersion isn't on the v10 API.
func (u *UniterAPIV10) CloudAPIVersion(_, _ struct{}) {}

// CloudAPIVersion returns the cloud API version, if available.
func (u *UniterAPI) CloudAPIVersion() (params.StringResult, error) {
	result := params.StringResult{}

	configGetter := stateenvirons.EnvironConfigGetter{Model: u.m, NewContainerBroker: u.containerBrokerFunc}
	spec, err := configGetter.CloudSpec()
	if err != nil {
		return result, common.ServerError(err)
	}
	apiVersion, err := configGetter.CloudAPIVersion(spec)
	if err != nil {
		return result, common.ServerError(err)
	}
	result.Result = apiVersion
	return result, err
}

// UpdateNetworkInfo refreshes the network settings for a unit's bound
// endpoints.
func (u *UniterAPI) UpdateNetworkInfo(args params.Entities) (params.ErrorResults, error) {
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}

	res := make([]params.ErrorResult, len(args.Entities))
	for i, entity := range args.Entities {
		unitTag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			res[i].Error = common.ServerError(err)
			continue
		}

		if !canAccess(unitTag) {
			res[i].Error = common.ServerError(common.ErrPerm)
			continue
		}

		if err = u.updateUnitNetworkInfo(unitTag); err != nil {
			res[i].Error = common.ServerError(err)
		}
	}

	return params.ErrorResults{Results: res}, nil
}

func (u *UniterAPI) updateUnitNetworkInfo(unitTag names.UnitTag) error {
	unit, err := u.getUnit(unitTag)
	if err != nil {
		return errors.Trace(err)
	}

	modelOp, err := u.updateUnitNetworkInfoOperation(unitTag, unit)
	if err != nil {
		return err
	}
	return u.st.ApplyOperation(modelOp)
}

func (u *UniterAPI) updateUnitNetworkInfoOperation(unitTag names.UnitTag, unit *state.Unit) (state.ModelOperation, error) {
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

		netInfo, err := NewNetworkInfo(u.st, unitTag)
		if err != nil {
			return nil, err
		}

		_, ingressAddresses, egressSubnets, err := netInfo.NetworksForRelation(relUnit.Endpoint().Name, rel, false)
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

// State isn't on the v14 API.
func (u *UniterAPIV14) State(_ struct{}) {}

// SetState isn't on the v14 API.
func (u *UniterAPIV14) SetState(_ struct{}) {}

// CommitHookChanges isn't on the v14 API.
func (u *UniterAPIV14) CommitHookChanges(_ struct{}) {}

// CommitHookChanges batches together all required API calls for applying
// a set of changes after a hook successfully completes and executes them in a
// single transaction.
func (u *UniterAPI) CommitHookChanges(args params.CommitHookChangesArgs) (params.ErrorResults, error) {
	canAccessUnit, err := u.accessUnit()
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}

	canAccessApp := makeAppAuthChecker(u.auth.GetAuthTag())

	res := make([]params.ErrorResult, len(args.Args))
	for i, arg := range args.Args {
		unitTag, err := names.ParseUnitTag(arg.Tag)
		if err != nil {
			res[i].Error = common.ServerError(err)
			continue
		}

		if !canAccessUnit(unitTag) {
			res[i].Error = common.ServerError(common.ErrPerm)
			continue
		}

		if err := u.commitHookChangesForOneUnit(unitTag, arg, canAccessUnit, canAccessApp); err != nil {
			// Log quota-related errors to aid operators
			if errors.IsQuotaLimitExceeded(err) {
				logger.Errorf("%s: %v", unitTag, err)
			}
			res[i].Error = common.ServerError(err)
		}
	}

	return params.ErrorResults{Results: res}, nil
}

func (u *UniterAPI) commitHookChangesForOneUnit(unitTag names.UnitTag, changes params.CommitHookChangesArg, canAccessUnit, canAccessApp common.AuthFunc) error {
	unit, err := u.getUnit(unitTag)
	if err != nil {
		return errors.Trace(err)
	}

	ctrlCfg, err := u.st.ControllerConfig()
	if err != nil {
		return errors.Trace(err)
	}

	appName, err := names.UnitApplication(unit.Name())
	if err != nil {
		return errors.Trace(err)
	}
	appTag := names.NewApplicationTag(appName).String()

	var modelOps []state.ModelOperation

	if changes.UpdateNetworkInfo {
		modelOp, err := u.updateUnitNetworkInfoOperation(unitTag, unit)
		if err != nil {
			return errors.Trace(err)
		}
		modelOps = append(modelOps, modelOp)
	}

	for _, rus := range changes.RelationUnitSettings {
		// Ensure the unit in the unit settings matches the root unit name
		if rus.Unit != changes.Tag {
			return common.ErrPerm
		}
		modelOp, err := u.updateUnitAndApplicationSettingsOp(rus, canAccessUnit)
		if err != nil {
			return errors.Trace(err)
		}
		modelOps = append(modelOps, modelOp)
	}

	if len(changes.OpenPorts)+len(changes.ClosePorts) > 0 {
		var openPortRanges, closePortRanges []corenetwork.PortRange
		for _, r := range changes.OpenPorts {
			// Ensure the tag in the port open request matches the root unit name
			if r.Tag != changes.Tag {
				return common.ErrPerm
			}
			openPortRanges = append(openPortRanges, corenetwork.PortRange{
				FromPort: r.FromPort,
				ToPort:   r.ToPort,
				Protocol: r.Protocol,
			})
		}
		for _, r := range changes.ClosePorts {
			// Ensure the tag in the port close request matches the root unit name
			if r.Tag != changes.Tag {
				return common.ErrPerm
			}
			closePortRanges = append(closePortRanges, corenetwork.PortRange{
				FromPort: r.FromPort,
				ToPort:   r.ToPort,
				Protocol: r.Protocol,
			})
		}

		// TODO(achilleas): we should be using endpoints instead of subnets
		// here. This emulates the existing behavior for the individual
		// Open/ClosePort API calls.
		modelOp, err := unit.OpenClosePortsOnSubnetOperation("", openPortRanges, closePortRanges)
		if err != nil {
			return errors.Trace(err)
		}
		modelOps = append(modelOps, modelOp)
	}

	if changes.SetUnitState != nil {
		// Ensure the tag in the set state request matches the root unit name
		if changes.SetUnitState.Tag != changes.Tag {
			return common.ErrPerm
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
		if changes.SetUnitState.MeterStatusState != nil {
			newUS.SetMeterStatusState(*changes.SetUnitState.MeterStatusState)
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
			return common.ErrPerm
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

	if changes.SetPodSpec != nil && changes.SetRawK8sSpec != nil {
		return errors.NewForbidden(nil, "either SetPodSpec or SetRawK8sSpec can be set for each application, but not both")
	}

	if changes.SetPodSpec != nil {
		// Ensure the application tag for the unit in the change arg
		// matches the one specified in the SetPodSpec payload.
		if changes.SetPodSpec.Tag != appTag {
			return errors.BadRequestf("application tag %q in SetPodSpec payload does not match the application for unit %q", changes.SetPodSpec.Tag, changes.Tag)
		}
		modelOp, err := u.setPodSpecOperation(changes.SetPodSpec.Tag, changes.SetPodSpec.Spec, unitTag, canAccessApp)
		if err != nil {
			return errors.Trace(err)
		}
		modelOps = append(modelOps, modelOp)
	}

	if changes.SetRawK8sSpec != nil {
		// Ensure the application tag for the unit in the change arg
		// matches the one specified in the SetRawK8sSpec payload.
		if changes.SetRawK8sSpec.Tag != appTag {
			return errors.BadRequestf("application tag %q in SetRawK8sSpec payload does not match the application for unit %q", changes.SetRawK8sSpec.Tag, changes.Tag)
		}
		modelOp, err := u.setRawK8sSpecOperation(changes.SetRawK8sSpec.Tag, changes.SetRawK8sSpec.Spec, unitTag, canAccessApp)
		if err != nil {
			return errors.Trace(err)
		}
		modelOps = append(modelOps, modelOp)
	}

	// Apply all changes in a single transaction.
	return u.st.ApplyOperation(state.ComposeModelOperations(modelOps...))
}
