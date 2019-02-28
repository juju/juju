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
	"github.com/juju/juju/state"
)

//go:generate mockgen -package mocks -destination mocks/facade_mock.go github.com/juju/juju/apiserver/facade Context,Resources,Authorizer
//go:generate mockgen -package mocks -destination mocks/instancemutater_mock.go github.com/juju/juju/apiserver/facades/agent/instancemutater InstanceMutaterState
//go:generate mockgen -package mocks -destination mocks/state_mock.go github.com/juju/juju/state EntityFinder,Entity,Lifer

var logger = loggo.GetLogger("juju.apiserver.instancemutater")

type InstanceMutaterAPIV1 interface {
	Life(args params.Entities) (params.LifeResults, error)
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

func NewInstanceMutaterFacade(ctx facade.Context) (*InstanceMutaterAPI, error) {
	st := &instanceMutaterStateShim{State: ctx.State()}
	return NewInstanceMutaterAPI(st, ctx.Resources(), ctx.Auth())
}

func NewInstanceMutaterAPI(st InstanceMutaterState, resources facade.Resources, authorizer facade.Authorizer) (*InstanceMutaterAPI, error) {
	if !authorizer.AuthMachineAgent() && !authorizer.AuthController() {
		return nil, common.ErrPerm
	}

	getAuthFunc := common.AuthFuncForMachineAgent(authorizer)
	getCanWatch := func() (common.AuthFunc, error) {
		return authorizer.AuthOwner, nil
	}
	return &InstanceMutaterAPI{
		ModelMachinesWatcher: common.NewModelMachinesWatcher(st, resources, authorizer),
		UnitsWatcher:         common.NewUnitsWatcher(st, resources, getCanWatch),
		LifeGetter:           common.NewLifeGetter(st, getAuthFunc),
		st:                   st,
		getAuthFunc:          getAuthFunc,
	}, nil
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
	return &machineShim{entity.(*state.Machine)}, nil
}

func (api *InstanceMutaterAPI) CharmProfilingInfo(arg params.ProfilingInfoArg) (params.ProfilingInfoResult, error) {
	result := params.ProfilingInfoResult{
		ProfileChanges: make([]params.ProfileChangeResult, len(arg.UnitNames)),
	}
	canAccess, err := api.getAuthFunc()
	if err != nil {
		return params.ProfilingInfoResult{}, common.ServerError(err)
	}
	m, err := api.getMachine(canAccess, names.NewMachineTag(arg.Entity.Tag))
	if err != nil {
		return params.ProfilingInfoResult{}, common.ServerError(err)
	}
	result, err = api.machineLXDProfileInfo(m, arg.UnitNames)
	if err != nil {
		return params.ProfilingInfoResult{}, common.ServerError(err)
	}
	return result, nil
}

func (api *InstanceMutaterAPI) machineLXDProfileInfo(m Machine, unitNames []string) (params.ProfilingInfoResult, error) {
	model, err := api.st.Model()
	if err != nil {
		return params.ProfilingInfoResult{}, errors.Trace(err)
	}
	modelName := model.Name()
	machineProfiles, err := m.CharmProfiles()
	if err != nil {
		return params.ProfilingInfoResult{}, errors.Trace(err)
	}
	result := params.ProfilingInfoResult{
		CurrentProfiles: machineProfiles,
		ProfileChanges:  make([]params.ProfileChangeResult, len(unitNames)),
	}

	for i, name := range unitNames {
		unit, err := api.st.Unit(name)
		if err != nil {
			return params.ProfilingInfoResult{}, errors.Trace(err)
		}
		app, err := unit.Application()
		if err != nil {
			return params.ProfilingInfoResult{}, errors.Trace(err)
		}
		ch, err := app.Charm()
		if err != nil {
			return params.ProfilingInfoResult{}, errors.Trace(err)
		}
		profile := ch.LXDProfile()
		var noProfile bool
		if profile == nil || (profile != nil && profile.Empty()) {
			noProfile = true
		}
		appName := app.Name()
		currentProfile, err := lxdprofile.MatchProfileNameByAppName(machineProfiles, appName)
		if err != nil {
			return params.ProfilingInfoResult{}, errors.Trace(err)
		}
		newProfile := lxdprofile.Name(modelName, appName, ch.Revision())
		// If the unit's charm has no profile, and no profile from the unit's
		// application is on the machine, or the machine's profile for the unit
		// matches the profile which would be created based on the unit's revision,
		// nothing to do here.
		if (noProfile && currentProfile == "") ||
			currentProfile == newProfile {
			continue
		}
		result.Changes = true
		result.ProfileChanges[i].OldProfileName = currentProfile
		result.ProfileChanges[i].NewProfileName = newProfile
		result.ProfileChanges[i].Profile = &params.CharmLXDProfile{
			Config:      profile.Config,
			Description: profile.Description,
			Devices:     profile.Devices,
		}
	}
	return result, nil
}
