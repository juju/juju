// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/core/status"
)

//go:generate mockgen -package mocks -destination mocks/facade_mock.go github.com/juju/juju/apiserver/facade Context,Resources,Authorizer
//go:generate mockgen -package mocks -destination mocks/instancemutater_mock.go github.com/juju/juju/apiserver/facades/agent/instancemutater InstanceMutaterState,Model,Machine,Unit,Application,Charm,LXDProfile
//go:generate mockgen -package mocks -destination mocks/modelcache_mock.go github.com/juju/juju/apiserver/facades/agent/instancemutater ModelCache,ModelCacheMachine
//go:generate mockgen -package mocks -destination mocks/state_mock.go github.com/juju/juju/state EntityFinder,Entity,Lifer
//go:generate mockgen -package mocks -destination mocks/watcher_mock.go github.com/juju/juju/core/cache NotifyWatcher,StringsWatcher

var logger = loggo.GetLogger("juju.apiserver.instancemutater")

// InstanceMutaterV1 defines the methods on the instance mutater API facade, version 1.
type InstanceMutaterV1 interface {
	WatchModelMachines() (params.StringsWatchResult, error)
	WatchUnits(args params.Entities) (params.StringsWatchResults, error)
	Life(args params.Entities) (params.LifeResults, error)

	CharmProfilingInfo(arg params.CharmProfilingInfoArg) (params.CharmProfilingInfoResult, error)
	SetUpgradeCharmProfileComplete(args params.SetProfileUpgradeCompleteArgs) (params.ErrorResults, error)
	SetCharmProfiles(args params.SetProfileArgs) (params.ErrorResults, error)
	WatchMachines() (params.StringsWatchResult, error)
	WatchApplicationLXDProfiles(args params.Entities) (params.NotifyWatchResults, error)
}

type InstanceMutaterAPI struct {
	*common.ModelMachinesWatcher
	*common.UnitsWatcher
	*common.LifeGetter

	st          InstanceMutaterState
	model       ModelCache
	resources   facade.Resources
	authorizer  facade.Authorizer
	getAuthFunc common.GetAuthFunc
}

// using apiserver/facades/client/cloud as an example.
var (
	_ InstanceMutaterV1 = (*InstanceMutaterAPI)(nil)
)

// NewFacadeV1 is used for API registration.
func NewFacadeV1(ctx facade.Context) (*InstanceMutaterAPI, error) {
	st := &instanceMutaterStateShim{State: ctx.State()}

	model, err := ctx.Controller().Model(st.ModelUUID())
	if err != nil {
		return nil, errors.Trace(err)
	}
	modelCache := &modelCacheShim{Model: model}

	return NewInstanceMutaterAPI(st, modelCache, ctx.Resources(), ctx.Auth())
}

// NewInstanceMutaterAPI creates a new API server endpoint for managing
// charm profiles on juju lxd machines and containers.
func NewInstanceMutaterAPI(st InstanceMutaterState,
	model ModelCache,
	resources facade.Resources,
	authorizer facade.Authorizer,
) (*InstanceMutaterAPI, error) {
	if !authorizer.AuthMachineAgent() && !authorizer.AuthController() {
		return nil, common.ErrPerm
	}

	getAuthFunc := common.AuthFuncForMachineAgent(authorizer)
	return &InstanceMutaterAPI{
		ModelMachinesWatcher: common.NewModelMachinesWatcher(st, resources, authorizer),
		UnitsWatcher:         common.NewUnitsWatcher(st, resources, getAuthFunc),
		LifeGetter:           common.NewLifeGetter(st, getAuthFunc),
		st:                   st,
		model:                model,
		resources:            resources,
		authorizer:           authorizer,
		getAuthFunc:          getAuthFunc,
	}, nil
}

// CharmProfilingInfo returns info to update lxd profiles on the machine
// based on the given unit names.  If the machine is not provisioned,
// no profile change info will be returned, nor will an error.
func (api *InstanceMutaterAPI) CharmProfilingInfo(arg params.CharmProfilingInfoArg) (params.CharmProfilingInfoResult, error) {
	result := params.CharmProfilingInfoResult{
		ProfileChanges: make([]params.ProfileChangeResult, len(arg.UnitNames)),
	}
	canAccess, err := api.getAuthFunc()
	if err != nil {
		return params.CharmProfilingInfoResult{}, errors.Trace(err)
	}
	tag, err := names.ParseMachineTag(arg.Entity.Tag)
	if err != nil {
		result.Error = common.ServerError(common.ErrPerm)
		return result, nil
	}
	m, err := api.getMachine(canAccess, tag)
	if err != nil {
		result.Error = common.ServerError(err)
		return result, nil
	}
	lxdProfileInfo, changes, err := api.machineLXDProfileInfo(m, arg.UnitNames)
	if err != nil {
		result.Error = common.ServerError(err)
	}

	// use the results from the machineLXDProfileInfo and apply them to the
	// result
	result.InstanceId = lxdProfileInfo.InstanceId
	result.ModelName = lxdProfileInfo.ModelName
	result.CurrentProfiles = lxdProfileInfo.MachineProfiles
	result.ProfileChanges = lxdProfileInfo.ProfileChanges
	result.Changes = changes

	return result, nil
}

// SetModificationStatus updates the instance whilst changes are occurring. This
// is different from SetStatus and SetInstanceStatus, by the fact this holds
// information about the ongoing changes that are happening to instances.
// Consider LXD Profile updates that can modify a instance, but may not cause
// the instance to be placed into a error state. This modification status
// serves the purpose of highlighting that to the operator.
// Only machine tags are accepted.
func (api *InstanceMutaterAPI) SetModificationStatus(args params.SetStatus) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	canAccess, err := api.getAuthFunc()
	if err != nil {
		logger.Errorf("failed to get an authorisation function: %v", err)
		return result, errors.Trace(err)
	}
	for i, arg := range args.Entities {
		err = api.setOneModificationStatus(canAccess, arg)
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// SetUpgradeCharmProfileComplete recorded that the result of updating
// the machine's charm profile(s)
func (api *InstanceMutaterAPI) SetUpgradeCharmProfileComplete(args params.SetProfileUpgradeCompleteArgs) (params.ErrorResults, error) {
	results := make([]params.ErrorResult, len(args.Args))
	canAccess, err := api.getAuthFunc()
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	for i, a := range args.Args {
		err := api.oneUpgradeCharmProfileComplete(a.Entity.Tag, a.UnitName, a.Message, canAccess)
		results[i].Error = common.ServerError(err)
	}
	return params.ErrorResults{Results: results}, nil
}

// SetCharmProfiles records the given slice of charm profile names.
func (api *InstanceMutaterAPI) SetCharmProfiles(args params.SetProfileArgs) (params.ErrorResults, error) {
	results := make([]params.ErrorResult, len(args.Args))
	canAccess, err := api.getAuthFunc()
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	for i, a := range args.Args {
		err := api.setOneMachineCharmProfiles(a.Entity.Tag, a.Profiles, canAccess)
		results[i].Error = common.ServerError(err)
	}
	return params.ErrorResults{Results: results}, nil
}

// WatchMachines starts a watcher to track machines.
// WatchMachines does not consume the initial event of the watch response, as
// that returns the initial set of machines that are currently available.
func (api *InstanceMutaterAPI) WatchMachines() (params.StringsWatchResult, error) {
	result := params.StringsWatchResult{}
	if !api.authorizer.AuthController() {
		return result, common.ErrPerm
	}

	watch := api.model.WatchMachines()
	if changes, ok := <-watch.Changes(); ok {
		result.StringsWatcherId = api.resources.Register(watch)
		result.Changes = changes
	} else {
		return result, errors.Errorf("cannot obtain initial model machines")
	}
	return result, nil
}

// WatchApplicationLXDProfiles starts a watcher to track Applications with
// LXD Profiles.
func (api *InstanceMutaterAPI) WatchApplicationLXDProfiles(args params.Entities) (params.NotifyWatchResults, error) {
	result := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	canAccess, err := api.getAuthFunc()
	if err != nil {
		return result, errors.Trace(err)
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseMachineTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		entityResult, err := api.watchOneEntityApplication(canAccess, tag)
		result.Results[i] = entityResult
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

func (api *InstanceMutaterAPI) watchOneEntityApplication(canAccess common.AuthFunc, tag names.MachineTag) (params.NotifyWatchResult, error) {
	result := params.NotifyWatchResult{}
	machine, err := api.getCacheMachine(canAccess, tag)
	if err != nil {
		return result, err
	}
	watch := machine.WatchApplicationLXDProfiles()
	// Consume the initial event before sending the result.
	if _, ok := <-watch.Changes(); ok {
		result.NotifyWatcherId = api.resources.Register(watch)
	} else {
		return result, errors.Errorf("cannot obtain initial machine watch application LXD profiles")
	}
	return result, nil
}

func (api *InstanceMutaterAPI) getCacheMachine(canAccess common.AuthFunc, tag names.MachineTag) (ModelCacheMachine, error) {
	if !canAccess(tag) {
		return nil, common.ErrPerm
	}
	machine, err := api.model.Machine(tag.Id())
	if err != nil {
		return nil, err
	}
	return machine, nil
}

func (api *InstanceMutaterAPI) getMachine(canAccess common.AuthFunc, tag names.MachineTag) (Machine, error) {
	if !canAccess(tag) {
		return nil, common.ErrPerm
	}
	entity, err := api.st.FindEntity(tag)
	if err != nil {
		return nil, err
	}
	// The authorization function guarantees that the tag represents a
	// machine.
	var machine Machine
	var ok bool
	if machine, ok = entity.(Machine); !ok {
		return nil, errors.NotValidf("machine entity")
	}
	return machine, nil
}

// lxdProfileInfo holds the profile information for the machineLXDProfileInfo
// to provide context to the result of the call.
type lxdProfileInfo struct {
	InstanceId      instance.Id
	ModelName       string
	MachineProfiles []string
	ProfileChanges  []params.ProfileChangeResult
}

func (api *InstanceMutaterAPI) machineLXDProfileInfo(m Machine, unitNames []string) (lxdProfileInfo, bool, error) {
	var empty lxdProfileInfo

	instId, err := m.InstanceId()
	if err != nil && params.IsCodeNotProvisioned(err) {
		// There is nothing we can do with this machine at this point. The
		// profiles will be applied when the machine is provisioned.
		logger.Tracef("Attempting to apply a profile to a machine that isn't provisioned %q", instId)
		return empty, false, nil
	}
	model, err := api.st.Model()
	if err != nil {
		return empty, false, errors.Trace(err)
	}
	modelName := model.Name()

	machineProfiles, err := m.CharmProfiles()
	if err != nil {
		return empty, false, errors.Trace(err)
	}
	changeResults := make([]params.ProfileChangeResult, len(unitNames))

	for i, name := range unitNames {
		unit, err := api.st.Unit(name)
		if err != nil {
			changeResults[i].Error = common.ServerError(err)
			continue
		}
		app, err := unit.Application()
		if err != nil {
			changeResults[i].Error = common.ServerError(err)
			continue
		}
		ch, err := app.Charm()
		if err != nil {
			changeResults[i].Error = common.ServerError(err)
			continue
		}
		profile := ch.LXDProfile()
		noProfile := !lxdprofile.NotEmpty(lxdCharmProfiler{
			Charm: ch,
		})
		appName := app.Name()
		currentProfile, err := lxdprofile.MatchProfileNameByAppName(machineProfiles, appName)
		if err != nil {
			changeResults[i].Error = common.ServerError(err)
			continue
		}
		newProfile := lxdprofile.Name(modelName, appName, ch.Revision())
		logger.Tracef("machineLXDProfileInfo noProfile(%t) currentProfile(%q), newProfile(%q)",
			noProfile, currentProfile, newProfile)
		// If the unit's charm has no profile, and no profile from the unit's
		// application is on the machine, or the machine's profile for the unit
		// matches the profile which would be created based on the unit's revision,
		// nothing to do here.
		if (noProfile && currentProfile == "") ||
			currentProfile == newProfile {
			continue
		}
		changeResults[i].OldProfileName = currentProfile
		changeResults[i].NewProfileName = newProfile
		changeResults[i].Profile = &params.CharmLXDProfile{
			Config:      profile.Config(),
			Description: profile.Description(),
			Devices:     profile.Devices(),
		}
	}
	return lxdProfileInfo{
		InstanceId:      instId,
		ModelName:       modelName,
		MachineProfiles: machineProfiles,
		ProfileChanges:  changeResults,
	}, true, nil
}

func (api *InstanceMutaterAPI) oneUpgradeCharmProfileComplete(machineTag, unitName, msg string, canAccess common.AuthFunc) error {
	mTag, err := names.ParseMachineTag(machineTag)
	if err != nil {
		return errors.Trace(err)
	}
	machine, err := api.getMachine(canAccess, mTag)
	if err != nil {
		return errors.Trace(err)
	}
	return machine.SetUpgradeCharmProfileComplete(unitName, msg)
}

func (api *InstanceMutaterAPI) setOneMachineCharmProfiles(machineTag string, profiles []string, canAccess common.AuthFunc) error {
	mTag, err := names.ParseMachineTag(machineTag)
	if err != nil {
		return errors.Trace(err)
	}
	machine, err := api.getMachine(canAccess, mTag)
	if err != nil {
		return errors.Trace(err)
	}
	return machine.SetCharmProfiles(profiles)
}

func (api *InstanceMutaterAPI) setOneModificationStatus(canAccess common.AuthFunc, arg params.EntityStatusArgs) error {
	logger.Tracef("SetInstanceStatus called with: %#v", arg)
	mTag, err := names.ParseMachineTag(arg.Tag)
	if err != nil {
		return common.ErrPerm
	}
	machine, err := api.getMachine(canAccess, mTag)
	if err != nil {
		logger.Debugf("SetModificationStatus unable to get machine %q", mTag)
		return err
	}

	// We can use the controller timestamp to get now.
	since, err := api.st.ControllerTimestamp()
	if err != nil {
		return err
	}
	s := status.StatusInfo{
		Status:  status.Status(arg.Status),
		Message: arg.Info,
		Data:    arg.Data,
		Since:   since,
	}
	if err = machine.SetModificationStatus(s); err != nil {
		logger.Debugf("failed to SetModificationStatus for %q: %v", mTag, err)
		return err
	}
	return nil
}
