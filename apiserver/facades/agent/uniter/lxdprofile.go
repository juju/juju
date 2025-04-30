// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	"github.com/juju/juju/apiserver/internal/charms"
	"github.com/juju/juju/core/instance"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/lxdprofile"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/unit"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type LXDProfileBackend interface {
	Machine(string) (LXDProfileMachine, error)
}

// LXDProfileMachine describes machine-receiver state methods
// for executing a lxd profile upgrade.
type LXDProfileMachine interface {
	ContainerType() instance.ContainerType
	IsManual() (bool, error)
}

type LXDProfileAPI struct {
	backend         LXDProfileBackend
	machineService  MachineService
	watcherRegistry facade.WatcherRegistry

	logger     corelogger.Logger
	accessUnit common.GetAuthFunc

	modelInfoService   ModelInfoService
	applicationService ApplicationService
}

// NewLXDProfileAPI returns a new LXDProfileAPI. Currently both
// GetAuthFuncs can used to determine current permissions.
func NewLXDProfileAPI(
	backend LXDProfileBackend,
	machineService MachineService,
	watcherRegistry facade.WatcherRegistry,
	authorizer facade.Authorizer,
	accessUnit common.GetAuthFunc,
	logger corelogger.Logger,
	modelInfoService ModelInfoService,
	applicationService ApplicationService,
) *LXDProfileAPI {
	return &LXDProfileAPI{
		backend:            backend,
		machineService:     machineService,
		watcherRegistry:    watcherRegistry,
		accessUnit:         accessUnit,
		logger:             logger,
		modelInfoService:   modelInfoService,
		applicationService: applicationService,
	}
}

// LXDProfileState implements the LXDProfileBackend indirection
// over state.State.
type LXDProfileState struct {
	st *state.State
}

func (s LXDProfileState) Machine(id string) (LXDProfileMachine, error) {
	m, err := s.st.Machine(id)
	return &lxdProfileMachine{m}, err
}

type lxdProfileMachine struct {
	*state.Machine
}

// NewExternalLXDProfileAPI can be used for API registration.
func NewExternalLXDProfileAPI(
	st *state.State,
	machineService MachineService,
	watcherRegistry facade.WatcherRegistry,
	authorizer facade.Authorizer,
	accessUnit common.GetAuthFunc,
	logger corelogger.Logger,
	modelInfoService ModelInfoService,
	applicationService ApplicationService,
) *LXDProfileAPI {
	return NewLXDProfileAPI(
		LXDProfileState{st},
		machineService,
		watcherRegistry,
		authorizer,
		accessUnit,
		logger,
		modelInfoService,
		applicationService,
	)
}

// WatchInstanceData returns a NotifyWatcher for observing
// changes to the lxd profile for one unit.
func (u *LXDProfileAPI) WatchInstanceData(ctx context.Context, args params.Entities) (params.NotifyWatchResults, error) {
	u.logger.Tracef(ctx, "Starting WatchInstanceData with %+v", args)
	result := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit(ctx)
	if err != nil {
		u.logger.Tracef(ctx, "WatchInstanceData error %+v", err)
		return params.NotifyWatchResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		if !canAccess(tag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		unitName, err := unit.NewName(tag.Id())
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		// TODO(nvinuesa): we could save this call if we move the lxd profile
		// watcher to the unit domain. Then, the watcher would be already
		// notifying for changes on the unit directly.
		machineUUID, err := u.applicationService.GetUnitMachineUUID(ctx, unitName)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		watcherId, err := u.watchOneInstanceData(ctx, machineUUID)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		result.Results[i].NotifyWatcherId = watcherId

	}
	u.logger.Tracef(ctx, "WatchInstanceData returning %+v", result)
	return result, nil
}

func (u *LXDProfileAPI) watchOneInstanceData(ctx context.Context, machineUUID coremachine.UUID) (string, error) {
	watcher, err := u.machineService.WatchLXDProfiles(ctx, machineUUID)
	if err != nil {
		return "", errors.Trace(err)
	}
	watcherID, _, err := internal.EnsureRegisterWatcher[struct{}](ctx, u.watcherRegistry, watcher)
	return watcherID, err
}

// LXDProfileName returns the name of the lxd profile applied to the unit's
// machine for the current charm version.
func (u *LXDProfileAPI) LXDProfileName(ctx context.Context, args params.Entities) (params.StringResults, error) {
	u.logger.Tracef(ctx, "Starting LXDProfileName with %+v", args)
	result := params.StringResults{
		Results: make([]params.StringResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit(ctx)
	if err != nil {
		return params.StringResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}

		if !canAccess(tag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		unitName, err := unit.NewName(tag.Id())
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		machineUUID, err := u.applicationService.GetUnitMachineUUID(ctx, unitName)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		name, err := u.getOneLXDProfileName(ctx, unitName.Application(), machineUUID)
		if errors.Is(err, machineerrors.NotProvisioned) {
			result.Results[i].Error = apiservererrors.ServerError(errors.NotProvisionedf("machine %q", machineUUID))
		} else if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		result.Results[i].Result = name

	}
	return result, nil
}

func (u *LXDProfileAPI) getOneLXDProfileName(ctx context.Context, appName string, machineUUID coremachine.UUID) (string, error) {
	profileNames, err := u.machineService.AppliedLXDProfileNames(ctx, machineUUID)
	if err != nil {
		u.logger.Errorf(ctx, "unable to retrieve LXD profiles for machine %q: %v", machineUUID, err)
		return "", err
	}
	return lxdprofile.MatchProfileNameByAppName(profileNames, appName)
}

// CanApplyLXDProfile returns true if
//   - this is an IAAS model,
//   - the unit is not on a manual machine,
//   - the provider type is "lxd" or it's an lxd container.
func (u *LXDProfileAPI) CanApplyLXDProfile(ctx context.Context, args params.Entities) (params.BoolResults, error) {
	u.logger.Tracef(ctx, "Starting CanApplyLXDProfile with %+v", args)
	result := params.BoolResults{
		Results: make([]params.BoolResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit(ctx)
	if err != nil {
		return params.BoolResults{}, errors.Trace(err)
	}
	modelInfo, err := u.modelInfoService.GetModelInfo(ctx)
	if err != nil {
		return params.BoolResults{}, errors.Trace(err)
	}
	if modelInfo.Type != model.IAAS {
		return result, nil
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}

		if !canAccess(tag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}

		name, err := u.getOneCanApplyLXDProfile(ctx, tag, modelInfo.CloudType)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		result.Results[i].Result = name
	}
	return result, nil
}

func (u *LXDProfileAPI) getOneCanApplyLXDProfile(ctx context.Context, tag names.Tag, providerType string) (bool, error) {
	unitName, err := unit.NewName(tag.Id())
	if err != nil {
		return false, internalerrors.Capture(err)
	}

	machineName, err := u.applicationService.GetUnitMachineName(ctx, unitName)
	if err != nil {
		return false, internalerrors.Capture(err)
	}

	machine, err := u.backend.Machine(machineName.String())
	if err != nil {
		return false, err
	}
	if manual, err := machine.IsManual(); err != nil {
		return false, err
	} else if manual {
		// We do no know what type of machine a manual one is, so we do not
		// manage lxd profiles on it.
		return false, nil
	}
	if providerType == "lxd" {
		return true, nil
	}
	switch machine.ContainerType() {
	case instance.LXD:
		return true, nil
	}
	return false, nil
}

// LXDProfileRequired returns true if charm has an lxd profile in it.
func (u *LXDProfileAPI) LXDProfileRequired(ctx context.Context, args params.CharmURLs) (params.BoolResults, error) {
	u.logger.Tracef(ctx, "Starting LXDProfileRequired with %+v", args)
	result := params.BoolResults{
		Results: make([]params.BoolResult, len(args.URLs)),
	}
	for i, arg := range args.URLs {
		required, err := u.getOneLXDProfileRequired(ctx, arg.URL)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		result.Results[i].Result = required
	}
	return result, nil
}

func (u *LXDProfileAPI) getOneLXDProfileRequired(ctx context.Context, curl string) (bool, error) {
	locator, err := charms.CharmLocatorFromURL(curl)
	if err != nil {
		return false, errors.Trace(err)
	}
	lxdProfile, _, err := u.applicationService.GetCharmLXDProfile(ctx, locator)
	if err != nil {
		return false, err
	}
	return !lxdProfile.Empty(), nil
}
