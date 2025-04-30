// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	apiservercharms "github.com/juju/juju/apiserver/internal/charms"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/logger"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// InstanceMutaterV2 defines the methods on the instance mutater API facade, version 2.
type InstanceMutaterV2 interface {
	Life(ctx context.Context, args params.Entities) (params.LifeResults, error)

	CharmProfilingInfo(ctx context.Context, arg params.Entity) (params.CharmProfilingInfoResult, error)
	ContainerType(ctx context.Context, arg params.Entity) (params.ContainerTypeResult, error)
	SetCharmProfiles(ctx context.Context, args params.SetProfileArgs) (params.ErrorResults, error)
	SetModificationStatus(ctx context.Context, args params.SetStatus) (params.ErrorResults, error)
	WatchMachines(ctx context.Context) (params.StringsWatchResult, error)
	WatchLXDProfileVerificationNeeded(ctx context.Context, args params.Entities) (params.NotifyWatchResults, error)
}

// MachineService defines the methods that the facade assumes from the Machine
// service.
type MachineService interface {
	// GetMachineUUID returns the UUID of a machine identified by its name.
	GetMachineUUID(ctx context.Context, name coremachine.Name) (coremachine.UUID, error)

	// InstanceID returns the cloud specific instance id for this machine.
	InstanceID(ctx context.Context, machineUUID coremachine.UUID) (instance.Id, error)

	// AppliedLXDProfileNames returns the names of the LXD profiles on the machine.
	AppliedLXDProfileNames(ctx context.Context, machineUUID coremachine.UUID) ([]string, error)

	// SetAppliedLXDProfileNames sets the list of LXD profile names to the
	// lxd_profile table for the given machine. This method will overwrite the list
	// of profiles for the given machine without any checks.
	SetAppliedLXDProfileNames(ctx context.Context, machineUUID coremachine.UUID, profileNames []string) error

	// WatchLXDProfiles returns a NotifyWatcher that is subscribed to the changes in
	// the machine_cloud_instance table in the model, for the given machine UUID.
	WatchLXDProfiles(ctx context.Context, machineUUID coremachine.UUID) (watcher.NotifyWatcher, error)
}

// ApplicationService is an interface for the application domain service.
type ApplicationService interface {
	// GetCharmLXDProfile returns the LXD profile along with the revision of the
	// charm using the charm name, source and revision.
	//
	// If the charm does not exist, a [applicationerrors.CharmNotFound] error is
	// returned.
	GetCharmLXDProfile(ctx context.Context, locator applicationcharm.CharmLocator) (charm.LXDProfile, applicationcharm.Revision, error)
	// WatchCharms returns a watcher that observes changes to charms.
	WatchCharms() (watcher.StringsWatcher, error)
}

// ModelInfoService provides access to information about the model.
type ModelInfoService interface {
	// GetModelInfo returns information about the current model.
	GetModelInfo(context.Context) (model.ModelInfo, error)
}

type InstanceMutaterAPI struct {
	*common.LifeGetter

	machineService     MachineService
	applicationService ApplicationService
	modelInfoService   ModelInfoService
	st                 InstanceMutaterState
	watcher            InstanceMutatorWatcher
	resources          facade.Resources
	authorizer         facade.Authorizer
	getAuthFunc        common.GetAuthFunc
	logger             logger.Logger
}

// InstanceMutatorWatcher instances return a lxd profile watcher for a machine.
type InstanceMutatorWatcher interface {
	WatchLXDProfileVerificationForMachine(context.Context, Machine, logger.Logger) (state.NotifyWatcher, error)
}

type instanceMutatorWatcher struct {
	st                 InstanceMutaterState
	machineService     MachineService
	applicationService ApplicationService
}

// NewInstanceMutaterAPI creates a new API server endpoint for managing
// charm profiles on juju lxd machines and containers.
func NewInstanceMutaterAPI(
	st InstanceMutaterState,
	machineService MachineService,
	applicationService ApplicationService,
	modelInfoService ModelInfoService,
	watcher InstanceMutatorWatcher,
	resources facade.Resources,
	authorizer facade.Authorizer,
	logger logger.Logger,
) *InstanceMutaterAPI {
	getAuthFunc := common.AuthFuncForMachineAgent(authorizer)
	return &InstanceMutaterAPI{
		LifeGetter:         common.NewLifeGetter(st, getAuthFunc),
		st:                 st,
		watcher:            watcher,
		resources:          resources,
		authorizer:         authorizer,
		getAuthFunc:        getAuthFunc,
		machineService:     machineService,
		applicationService: applicationService,
		modelInfoService:   modelInfoService,
		logger:             logger,
	}
}

// CharmProfilingInfo returns info to update lxd profiles on the machine. If
// the machine is not provisioned, no profile change info will be returned,
// nor will an error.
func (api *InstanceMutaterAPI) CharmProfilingInfo(ctx context.Context, arg params.Entity) (params.CharmProfilingInfoResult, error) {
	result := params.CharmProfilingInfoResult{
		ProfileChanges: make([]params.ProfileInfoResult, 0),
	}
	canAccess, err := api.getAuthFunc(ctx)
	if err != nil {
		return params.CharmProfilingInfoResult{}, errors.Trace(err)
	}
	tag, err := names.ParseMachineTag(arg.Tag)
	if err != nil {
		result.Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
		return result, nil
	}
	m, err := api.getMachine(canAccess, tag)
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result, nil
	}
	lxdProfileInfo, err := api.machineLXDProfileInfo(ctx, m)
	if errors.Is(err, machineerrors.NotProvisioned) {
		result.Error = apiservererrors.ServerError(errors.NotProvisionedf("machine %q", tag.Id()))
	} else if err != nil {
		result.Error = apiservererrors.ServerError(errors.Annotatef(err, "%s", tag))
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
func (api *InstanceMutaterAPI) ContainerType(ctx context.Context, arg params.Entity) (params.ContainerTypeResult, error) {
	result := params.ContainerTypeResult{}
	canAccess, err := api.getAuthFunc(ctx)
	if err != nil {
		return result, errors.Trace(err)
	}
	tag, err := names.ParseMachineTag(arg.Tag)
	if err != nil {
		result.Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
		return result, nil
	}
	m, err := api.getMachine(canAccess, tag)
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
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
func (api *InstanceMutaterAPI) SetModificationStatus(ctx context.Context, args params.SetStatus) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	canAccess, err := api.getAuthFunc(ctx)
	if err != nil {
		api.logger.Errorf(ctx, "failed to get an authorisation function: %v", err)
		return result, errors.Trace(err)
	}
	for i, arg := range args.Entities {
		err = api.setOneModificationStatus(ctx, canAccess, arg)
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

// SetCharmProfiles records the given slice of charm profile names.
func (api *InstanceMutaterAPI) SetCharmProfiles(ctx context.Context, args params.SetProfileArgs) (params.ErrorResults, error) {
	results := make([]params.ErrorResult, len(args.Args))
	canAccess, err := api.getAuthFunc(ctx)
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	for i, a := range args.Args {
		err := api.setOneMachineCharmProfiles(ctx, a.Entity.Tag, a.Profiles, canAccess)
		if errors.Is(err, machineerrors.NotProvisioned) {
			results[i].Error = apiservererrors.ServerError(errors.NotProvisionedf("machine %q", a.Entity.Tag))
		} else if err != nil {
			results[i].Error = apiservererrors.ServerError(err)
		}
	}
	return params.ErrorResults{Results: results}, nil
}

// WatchMachines starts a watcher to track machines.
// WatchMachines does not consume the initial event of the watch response, as
// that returns the initial set of machines that are currently available.
func (api *InstanceMutaterAPI) WatchMachines(ctx context.Context) (params.StringsWatchResult, error) {
	result := params.StringsWatchResult{}
	if !api.authorizer.AuthController() {
		return result, apiservererrors.ErrPerm
	}

	watch := api.st.WatchMachines()
	if changes, ok := <-watch.Changes(); ok {
		result.StringsWatcherId = api.resources.Register(watch)
		result.Changes = changes
	} else {
		return result, errors.Errorf("cannot obtain initial model machines")
	}
	return result, nil
}

// WatchModelMachines starts a watcher to track machines, but not containers.
// WatchModelMachines does not consume the initial event of the watch response, as
// that returns the initial set of machines that are currently available.
func (api *InstanceMutaterAPI) WatchModelMachines(ctx context.Context) (params.StringsWatchResult, error) {
	result := params.StringsWatchResult{}
	if !api.authorizer.AuthController() {
		return result, apiservererrors.ErrPerm
	}

	watch := api.st.WatchModelMachines()
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
func (api *InstanceMutaterAPI) WatchContainers(ctx context.Context, arg params.Entity) (params.StringsWatchResult, error) {
	result := params.StringsWatchResult{}
	canAccess, err := api.getAuthFunc(ctx)
	if err != nil {
		return result, errors.Trace(err)
	}
	tag, err := names.ParseMachineTag(arg.Tag)
	if err != nil {
		return result, errors.Trace(err)
	}
	machine, err := api.getMachine(canAccess, tag)
	if err != nil {
		return result, err
	}
	watch := machine.WatchContainers(instance.LXD)
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
func (api *InstanceMutaterAPI) WatchLXDProfileVerificationNeeded(ctx context.Context, args params.Entities) (params.NotifyWatchResults, error) {
	result := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	canAccess, err := api.getAuthFunc(ctx)
	if err != nil {
		return result, errors.Trace(err)
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseMachineTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		entityResult, err := api.watchOneEntityApplication(ctx, canAccess, tag)
		result.Results[i] = entityResult
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func (api *InstanceMutaterAPI) watchOneEntityApplication(ctx context.Context, canAccess common.AuthFunc, tag names.MachineTag) (params.NotifyWatchResult, error) {
	result := params.NotifyWatchResult{}
	machine, err := api.getMachine(canAccess, tag)
	if err != nil {
		return result, err
	}
	isManual, err := machine.IsManual()
	if err != nil {
		return result, errors.Trace(err)
	}
	if isManual {
		return result, errors.NotSupportedf("watching lxd profiles on manual machines")
	}
	watch, err := api.watcher.WatchLXDProfileVerificationForMachine(ctx, machine, api.logger)
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

// WatchLXDProfileVerificationForMachine notifies if any of the following happen
// relative to the specified machine:
//  1. A new unit whose charm has an LXD profile is added.
//  2. A unit being removed has a profile and other units
//     exist on the machine.
//  3. The LXD profile of an application with a unit on the
//     machine is added, removed, or exists. This also includes scenarios
//     where the charm is being downloaded asynchronously and its metadata
//     gets updated once the download is complete.
//  4. The machine's instanceId is changed, indicating it
//     has been provisioned.
func (w *instanceMutatorWatcher) WatchLXDProfileVerificationForMachine(ctx context.Context, machine Machine, logger logger.Logger) (state.NotifyWatcher, error) {
	return newMachineLXDProfileWatcher(
		ctx,
		MachineLXDProfileWatcherConfig{
			machine:            machine,
			backend:            w.st,
			machineService:     w.machineService,
			applicationService: w.applicationService,
			logger:             logger,
		},
	)
}

func (api *InstanceMutaterAPI) getMachine(canAccess common.AuthFunc, tag names.MachineTag) (Machine, error) {
	if !canAccess(tag) {
		return nil, apiservererrors.ErrPerm
	}
	return api.st.Machine(tag.Id())
}

// lxdProfileInfo holds the profile information for the machineLXDProfileInfo
// to provide context to the result of the call.
type lxdProfileInfo struct {
	InstanceId      instance.Id
	ModelName       string
	MachineProfiles []string
	ProfileUnits    []params.ProfileInfoResult
}

func (api *InstanceMutaterAPI) machineLXDProfileInfo(ctx context.Context, m Machine) (lxdProfileInfo, error) {
	var empty lxdProfileInfo

	machineUUID, err := api.machineService.GetMachineUUID(ctx, coremachine.Name(m.Id()))
	if err != nil {
		return empty, errors.Trace(err)
	}
	instId, err := api.machineService.InstanceID(ctx, machineUUID)
	if err != nil {
		return empty, errors.Trace(errors.Annotate(err, "attempting to get instanceId"))
	}

	units, err := m.Units()
	if err != nil {
		return empty, errors.Trace(err)
	}
	machineProfiles, err := api.machineService.AppliedLXDProfileNames(ctx, machineUUID)
	if err != nil {
		return empty, errors.Trace(err)
	}

	var changeResults []params.ProfileInfoResult
	for _, unit := range units {
		if unit.Life() == state.Dead {
			api.logger.Debugf(ctx, "unit %q is dead, do not load profile", unit.Name())
			continue
		}
		appName := unit.ApplicationName()
		app, err := api.st.Application(appName)
		if err != nil {
			changeResults = append(changeResults, params.ProfileInfoResult{
				Error: apiservererrors.ServerError(err)})
			continue
		}
		charmURLStr := app.CharmURL()
		// Defensive check, shouldn't be nil.
		if charmURLStr == nil {
			continue
		}

		locator, err := apiservercharms.CharmLocatorFromURL(*charmURLStr)
		if err != nil {
			return empty, errors.Trace(err)
		}

		lxdProfile, _, err := api.applicationService.GetCharmLXDProfile(ctx, locator)
		if errors.Is(err, applicationerrors.CharmNotFound) {
			return empty, errors.NotFoundf("charm %q", *charmURLStr)
		} else if err != nil {
			changeResults = append(changeResults, params.ProfileInfoResult{
				Error: apiservererrors.ServerError(err)})
			continue
		}

		var normalised *params.CharmLXDProfile
		if profile := lxdProfile; !profile.Empty() {
			normalised = &params.CharmLXDProfile{
				Config:      profile.Config,
				Description: profile.Description,
				Devices:     profile.Devices,
			}
		}
		changeResults = append(changeResults, params.ProfileInfoResult{
			ApplicationName: appName,
			Revision:        locator.Revision,
			Profile:         normalised,
		})
	}
	modelInfo, err := api.modelInfoService.GetModelInfo(ctx)
	if err != nil {
		return empty, errors.Trace(err)
	}
	return lxdProfileInfo{
		InstanceId:      instId,
		ModelName:       modelInfo.Name,
		MachineProfiles: machineProfiles,
		ProfileUnits:    changeResults,
	}, nil
}

func (api *InstanceMutaterAPI) setOneMachineCharmProfiles(ctx context.Context, machineTag string, profiles []string, canAccess common.AuthFunc) error {
	mTag, err := names.ParseMachineTag(machineTag)
	if err != nil {
		return errors.Trace(err)
	}
	if !canAccess(mTag) {
		return apiservererrors.ErrPerm
	}

	machineUUID, err := api.machineService.GetMachineUUID(ctx, coremachine.Name(mTag.Id()))
	if err != nil {
		api.logger.Errorf(ctx, "getting machine uuid: %w", err)
		return errors.Trace(err)
	}
	return errors.Trace(api.machineService.SetAppliedLXDProfileNames(ctx, machineUUID, profiles))
}

func (api *InstanceMutaterAPI) setOneModificationStatus(ctx context.Context, canAccess common.AuthFunc, arg params.EntityStatusArgs) error {
	api.logger.Tracef(ctx, "SetInstanceStatus called with: %#v", arg)
	mTag, err := names.ParseMachineTag(arg.Tag)
	if err != nil {
		return apiservererrors.ErrPerm
	}
	machine, err := api.getMachine(canAccess, mTag)
	if err != nil {
		api.logger.Debugf(ctx, "SetModificationStatus unable to get machine %q", mTag)
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
		api.logger.Debugf(ctx, "failed to SetModificationStatus for %q: %v", mTag, err)
		return err
	}
	return nil
}
