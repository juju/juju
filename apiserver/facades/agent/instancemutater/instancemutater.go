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
	"github.com/juju/juju/core/lxdprofile"
)

//go:generate mockgen -package mocks -destination mocks/facade_mock.go github.com/juju/juju/apiserver/facade Context,Resources,Authorizer
//go:generate mockgen -package mocks -destination mocks/instancemutater_mock.go github.com/juju/juju/apiserver/facades/agent/instancemutater InstanceMutaterState,Model,Machine,Unit,Application,Charm,LXDProfile
//go:generate mockgen -package mocks -destination mocks/state_mock.go github.com/juju/juju/state EntityFinder,Entity,Lifer

var logger = loggo.GetLogger("juju.apiserver.instancemutater")

// InstanceMutaterV1 defines the methods on the instance mutater API facade, version 1.
type InstanceMutaterV1 interface {
	CharmProfilingInfo(params.CharmProfilingInfoArg) (params.CharmProfilingInfoResult, error)
	Life(args params.Entities) (params.LifeResults, error)
	SetCharmProfiles(params.SetProfileArgs) (params.ErrorResults, error)
	SetUpgradeCharmProfileComplete(params.SetProfileUpgradeCompleteArgs) (params.ErrorResults, error)
	WatchUnits(args params.Entities) (params.StringsWatchResults, error)
	WatchModelMachines() (params.StringsWatchResult, error)
}

type InstanceMutaterAPI struct {
	*common.ModelMachinesWatcher
	*common.UnitsWatcher
	*common.LifeGetter

	st          InstanceMutaterState
	getAuthFunc common.GetAuthFunc
}

// using apiserver/facades/client/cloud as an example.
var (
	_ InstanceMutaterV1 = (*InstanceMutaterAPI)(nil)
)

// NewFacadeV1 is used for API registration.
func NewFacadeV1(ctx facade.Context) (*InstanceMutaterAPI, error) {
	st := &instanceMutaterStateShim{State: ctx.State()}
	return NewInstanceMutaterAPI(st, ctx.Resources(), ctx.Auth())
}

// NewInstanceMutaterAPI creates a new API server endpoint for managing
// charm profiles on juju lxd machines and containers.
func NewInstanceMutaterAPI(st InstanceMutaterState, resources facade.Resources, authorizer facade.Authorizer) (*InstanceMutaterAPI, error) {
	if !authorizer.AuthMachineAgent() && !authorizer.AuthController() {
		return nil, common.ErrPerm
	}

	getAuthFunc := common.AuthFuncForMachineAgent(authorizer)
	return &InstanceMutaterAPI{
		ModelMachinesWatcher: common.NewModelMachinesWatcher(st, resources, authorizer),
		UnitsWatcher:         common.NewUnitsWatcher(st, resources, getAuthFunc),
		LifeGetter:           common.NewLifeGetter(st, getAuthFunc),
		st:                   st,
		getAuthFunc:          getAuthFunc,
	}, nil
}

// CharmProfilingInfo returns info to update lxd profiles on the machine
// based on the given unit names.
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
	result.ProfileChanges, result.CurrentProfiles, result.Changes, err = api.machineLXDProfileInfo(m, arg.UnitNames)
	if err != nil {
		result.Error = common.ServerError(err)
	}

	return result, nil
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

func (api *InstanceMutaterAPI) machineLXDProfileInfo(m Machine, unitNames []string) ([]params.ProfileChangeResult, []string, bool, error) {
	if instId, err := m.InstanceId(); err != nil && params.IsCodeNotProvisioned(err) {
		// There is nothing we can do with this machine at this point. The
		// profiles will be applied when the machine is provisioned.
		logger.Tracef("Attempting to apply a profile to a machine that isn't provisioned %q", instId)
		return nil, nil, false, nil
	}
	model, err := api.st.Model()
	if err != nil {
		return nil, nil, false, errors.Trace(err)
	}
	modelName := model.Name()

	machineProfiles, err := m.CharmProfiles()
	if err != nil {
		return nil, machineProfiles, false, errors.Trace(err)
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
	return changeResults, machineProfiles, true, nil
}

// SetUpgradeCharmProfileComplete recorded that the result of updating
// the machine's charm profile(s)
func (api *InstanceMutaterAPI) SetUpgradeCharmProfileComplete(args params.SetProfileUpgradeCompleteArgs) (params.ErrorResults, error) {
	results := make([]params.ErrorResult, len(args.Args))
	canAccess, err := api.getAuthFunc()
	if err != nil {
		logger.Errorf("failed to get an authorisation function: %v", err)
		return params.ErrorResults{}, errors.Trace(err)
	}
	for i, a := range args.Args {
		results[i].Error = common.ServerError(api.oneUpgradeCharmProfileComplete(a.Entity.Tag, a.UnitName, a.Message, canAccess))
	}
	return params.ErrorResults{Results: results}, nil
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

// SetCharmProfiles records the given slice of charm profile names.
func (api *InstanceMutaterAPI) SetCharmProfiles(args params.SetProfileArgs) (params.ErrorResults, error) {
	results := make([]params.ErrorResult, len(args.Args))
	canAccess, err := api.getAuthFunc()
	if err != nil {
		logger.Errorf("failed to get an authorisation function: %v", err)
		return params.ErrorResults{}, errors.Trace(err)
	}
	for i, a := range args.Args {
		results[i].Error = common.ServerError(api.setOneMachineCharmProfiles(a.Entity.Tag, a.Profiles, canAccess))
	}
	return params.ErrorResults{Results: results}, nil
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
