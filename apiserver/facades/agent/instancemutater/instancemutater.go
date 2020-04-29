// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater

import (
	"github.com/juju/errors"
	"github.com/juju/juju/state"
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/status"
)

//go:generate mockgen -package mocks -destination mocks/instancemutater_mock.go github.com/juju/juju/apiserver/facades/agent/instancemutater InstanceMutaterState,Machine,Unit,Application,Charm
//go:generate mockgen -package mocks -destination mocks/modelcache_mock.go github.com/juju/juju/apiserver/facades/agent/instancemutater ModelCache,ModelCacheMachine
//go:generate mockgen -package mocks -destination mocks/state_mock.go github.com/juju/juju/state EntityFinder,Entity,Lifer
//go:generate mockgen -package mocks -destination mocks/watcher_mock.go github.com/juju/juju/core/cache NotifyWatcher,StringsWatcher

var logger = loggo.GetLogger("juju.apiserver.instancemutater")

// InstanceMutaterV1 defines the methods on the instance mutater API facade, version 1.
type InstanceMutaterV1 interface {
	Life(args params.Entities) (params.LifeResults, error)

	CharmProfilingInfo(arg params.Entity) (params.CharmProfilingInfoResult, error)
	SetCharmProfiles(args params.SetProfileArgs) (params.ErrorResults, error)
	SetModificationStatus(args params.SetStatus) (params.ErrorResults, error)
	WatchMachines() (params.StringsWatchResult, error)
	WatchLXDProfileVerificationNeeded(args params.Entities) (params.NotifyWatchResults, error)
}

// InstanceMutaterV2 defines the methods on the instance mutater API facade, version 2.
type InstanceMutaterV2 interface {
	Life(args params.Entities) (params.LifeResults, error)

	CharmProfilingInfo(arg params.Entity) (params.CharmProfilingInfoResult, error)
	ContainerType(arg params.Entity) (params.ContainerTypeResult, error)
	SetCharmProfiles(args params.SetProfileArgs) (params.ErrorResults, error)
	SetModificationStatus(args params.SetStatus) (params.ErrorResults, error)
	WatchMachines() (params.StringsWatchResult, error)
	WatchLXDProfileVerificationNeeded(args params.Entities) (params.NotifyWatchResults, error)
}

type InstanceMutaterAPI struct {
	*common.LifeGetter

	st          InstanceMutaterState
	model       ModelCache
	resources   facade.Resources
	authorizer  facade.Authorizer
	getAuthFunc common.GetAuthFunc
	machineFunc EntityMachineFunc
}

type EntityMachineFunc func(state.Entity) (Machine, error)

type InstanceMutaterAPIV1 struct {
	*InstanceMutaterAPI
}

// using apiserver/facades/client/cloud as an example.
var (
	_ InstanceMutaterV2 = (*InstanceMutaterAPI)(nil)
	_ InstanceMutaterV1 = (*InstanceMutaterAPIV1)(nil)
)

// NewFacadeV2 is used for API registration.
func NewFacadeV2(ctx facade.Context) (*InstanceMutaterAPI, error) {
	st := &instanceMutaterStateShim{State: ctx.State()}

	model, err := ctx.Controller().Model(st.ModelUUID())
	if err != nil {
		return nil, errors.Trace(err)
	}
	modelCache := &modelCacheShim{Model: model}

	return NewInstanceMutaterAPI(st, modelCache, ctx.Resources(), ctx.Auth())
}

// NewFacadeV1 is used for API registration.
func NewFacadeV1(ctx facade.Context) (*InstanceMutaterAPIV1, error) {
	v2, err := NewFacadeV2(ctx)
	if err != nil {
		return nil, err
	}
	return &InstanceMutaterAPIV1{v2}, nil
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
		LifeGetter:  common.NewLifeGetter(st, getAuthFunc),
		st:          st,
		model:       model,
		resources:   resources,
		authorizer:  authorizer,
		getAuthFunc: getAuthFunc,
		machineFunc: machineFromEntity,
	}, nil
}

// CharmProfilingInfo returns info to update lxd profiles on the machine. If
// the machine is not provisioned, no profile change info will be returned,
// nor will an error.
func (api *InstanceMutaterAPI) CharmProfilingInfo(arg params.Entity) (params.CharmProfilingInfoResult, error) {
	result := params.CharmProfilingInfoResult{
		ProfileChanges: make([]params.ProfileInfoResult, 0),
	}
	canAccess, err := api.getAuthFunc()
	if err != nil {
		return params.CharmProfilingInfoResult{}, errors.Trace(err)
	}
	tag, err := names.ParseMachineTag(arg.Tag)
	if err != nil {
		result.Error = common.ServerError(common.ErrPerm)
		return result, nil
	}
	m, err := api.getMachine(canAccess, tag)
	if err != nil {
		result.Error = common.ServerError(err)
		return result, nil
	}
	lxdProfileInfo, err := api.machineLXDProfileInfo(m)
	if err != nil {
		result.Error = common.ServerError(errors.Annotatef(err, "%s", tag))
	}

	// use the results from the machineLXDProfileInfo and apply them to the
	// result
	result.InstanceId = lxdProfileInfo.InstanceId
	result.ModelName = lxdProfileInfo.ModelName
	result.CurrentProfiles = lxdProfileInfo.MachineProfiles
	result.ProfileChanges = lxdProfileInfo.ProfileUnits

	return result, nil
}

// ContainerType returns the container type of a machine.
func (api *InstanceMutaterAPI) ContainerType(arg params.Entity) (params.ContainerTypeResult, error) {
	result := params.ContainerTypeResult{}
	canAccess, err := api.getAuthFunc()
	if err != nil {
		return result, errors.Trace(err)
	}
	tag, err := names.ParseMachineTag(arg.Tag)
	if err != nil {
		result.Error = common.ServerError(common.ErrPerm)
		return result, nil
	}
	m, err := api.getCacheMachine(canAccess, tag)
	if err != nil {
		result.Error = common.ServerError(err)
		return result, nil
	}
	result.Type = m.ContainerType()
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

	watch, err := api.model.WatchMachines()
	if err != nil {
		return result, err
	}
	if changes, ok := <-watch.Changes(); ok {
		result.StringsWatcherId = api.resources.Register(watch)
		result.Changes = changes
	} else {
		return result, errors.Errorf("cannot obtain initial model machines")
	}
	return result, nil
}

// WatchContainers starts a watcher to track Containers on a given
// machine.
func (api *InstanceMutaterAPI) WatchContainers(arg params.Entity) (params.StringsWatchResult, error) {
	result := params.StringsWatchResult{}
	canAccess, err := api.getAuthFunc()
	if err != nil {
		return result, errors.Trace(err)
	}
	tag, err := names.ParseMachineTag(arg.Tag)
	if err != nil {
		return result, errors.Trace(err)
	}
	machine, err := api.getCacheMachine(canAccess, tag)
	if err != nil {
		return result, err
	}
	watch, err := machine.WatchContainers()
	if err != nil {
		return result, err
	}
	if changes, ok := <-watch.Changes(); ok {
		result.StringsWatcherId = api.resources.Register(watch)
		result.Changes = changes
	} else {
		return result, errors.Errorf("cannot obtain initial machine containers")
	}
	return result, nil
}

// WatchLXDProfileVerificationNeeded starts a watcher to track Applications with
// LXD Profiles.
func (api *InstanceMutaterAPI) WatchLXDProfileVerificationNeeded(args params.Entities) (params.NotifyWatchResults, error) {
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
	watch, err := machine.WatchLXDProfileVerificationNeeded()
	if err != nil {
		return result, err
	}
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
	// call a function, default to below... or pass in a func for the mocks to work.
	// separate constructor in export
	return api.machineFunc(entity)
}

func machineFromEntity(entity state.Entity) (Machine, error) {
	var m *state.Machine
	var ok bool
	if m, ok = entity.(*state.Machine); !ok {
		return nil, errors.NotValidf("machine entity")
	}
	return &machineShim{m}, nil
}

// lxdProfileInfo holds the profile information for the machineLXDProfileInfo
// to provide context to the result of the call.
type lxdProfileInfo struct {
	InstanceId      instance.Id
	ModelName       string
	MachineProfiles []string
	ProfileUnits    []params.ProfileInfoResult
}

func (api *InstanceMutaterAPI) machineLXDProfileInfo(m Machine) (lxdProfileInfo, error) {
	var empty lxdProfileInfo

	instId, err := m.InstanceId()
	if err != nil {
		return empty, errors.Trace(errors.Annotate(err, "attempting to get instanceId"))
	}

	units, err := m.Units()
	if err != nil {
		return empty, errors.Trace(err)
	}
	machineProfiles, err := m.CharmProfiles()
	if err != nil {
		return empty, errors.Trace(err)
	}
	changeResults := make([]params.ProfileInfoResult, len(units))
	for i, unit := range units {
		appName := unit.Application()
		app, err := api.st.Application(appName)
		if err != nil {
			changeResults[i].Error = common.ServerError(err)
			continue
		}
		chURL := app.CharmURL()
		ch, err := api.st.Charm(chURL)
		if err != nil {
			changeResults[i].Error = common.ServerError(err)
			continue
		}

		var normalised *params.CharmLXDProfile
		if profile := ch.LXDProfile(); !profile.Empty() {
			normalised = &params.CharmLXDProfile{
				Config:      profile.Config,
				Description: profile.Description,
				Devices:     profile.Devices,
			}
		}
		changeResults[i] = params.ProfileInfoResult{
			ApplicationName: appName,
			Revision:        chURL.Revision,
			Profile:         normalised,
		}
	}
	return lxdProfileInfo{
		InstanceId:      instId,
		ModelName:       api.model.Name(),
		MachineProfiles: machineProfiles,
		ProfileUnits:    changeResults,
	}, nil
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
