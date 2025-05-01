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
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type LXDProfileBackendV2 interface {
	Machine(string) (LXDProfileMachineV2, error)
	Unit(string) (LXDProfileUnitV2, error)
}

// LXDProfileMachineV2 describes machine-receiver state methods
// for executing a lxd profile upgrade.
type LXDProfileMachineV2 interface {
	ContainerType() instance.ContainerType
	IsManual() (bool, error)
}

// LXDProfileUnitV2 describes unit-receiver state methods
// for executing a lxd profile upgrade.
type LXDProfileUnitV2 interface {
	ApplicationName() string
	AssignedMachineId() (string, error)
	CharmURL() *string
	Name() string
	Tag() names.Tag
}

// LXDProfileCharmV2 describes charm-receiver state methods
// for executing a lxd profile upgrade.
type LXDProfileCharmV2 interface {
	LXDProfile() lxdprofile.Profile
}

type LXDProfileAPIv2 struct {
	backend         LXDProfileBackendV2
	machineService  MachineService
	watcherRegistry facade.WatcherRegistry

	logger     corelogger.Logger
	accessUnit common.GetAuthFunc

	modelInfoService   ModelInfoService
	applicationService ApplicationService
}

// NewLXDProfileAPIv2 returns a new LXDProfileAPIv2. Currently both
// GetAuthFuncs can used to determine current permissions.
func NewLXDProfileAPIv2(
	backend LXDProfileBackendV2,
	machineService MachineService,
	watcherRegistry facade.WatcherRegistry,
	authorizer facade.Authorizer,
	accessUnit common.GetAuthFunc,
	logger corelogger.Logger,
	modelInfoService ModelInfoService,
	applicationService ApplicationService,
) *LXDProfileAPIv2 {
	return &LXDProfileAPIv2{
		backend:            backend,
		machineService:     machineService,
		watcherRegistry:    watcherRegistry,
		accessUnit:         accessUnit,
		logger:             logger,
		modelInfoService:   modelInfoService,
		applicationService: applicationService,
	}
}

// LXDProfileStateV2 implements the LXDProfileBackendV2 indirection
// over state.State.
type LXDProfileStateV2 struct {
	st *state.State
}

func (s LXDProfileStateV2) Machine(id string) (LXDProfileMachineV2, error) {
	m, err := s.st.Machine(id)
	return &lxdProfileMachineV2{m}, err
}

func (s LXDProfileStateV2) Unit(id string) (LXDProfileUnitV2, error) {
	return s.st.Unit(id)
}

type lxdProfileMachineV2 struct {
	*state.Machine
}

// NewExternalLXDProfileAPIv2 can be used for API registration.
func NewExternalLXDProfileAPIv2(
	st *state.State,
	machineService MachineService,
	watcherRegistry facade.WatcherRegistry,
	authorizer facade.Authorizer,
	accessUnit common.GetAuthFunc,
	logger corelogger.Logger,
	modelInfoService ModelInfoService,
	applicationService ApplicationService,
) *LXDProfileAPIv2 {
	return NewLXDProfileAPIv2(
		LXDProfileStateV2{st},
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
func (u *LXDProfileAPIv2) WatchInstanceData(ctx context.Context, args params.Entities) (params.NotifyWatchResults, error) {
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
		unit, err := u.backend.Unit(tag.Id())
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		// TODO(nvinuesa): we could save this call if we move the lxd profile
		// watcher to the unit domain. Then, the watcher would be already
		// notifying for changes on the unit directly.
		machineID, err := unit.AssignedMachineId()
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		watcherId, err := u.watchOneInstanceData(ctx, machineID)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		result.Results[i].NotifyWatcherId = watcherId

	}
	u.logger.Tracef(ctx, "WatchInstanceData returning %+v", result)
	return result, nil
}

func (u *LXDProfileAPIv2) watchOneInstanceData(ctx context.Context, machineID string) (string, error) {
	machineUUID, err := u.machineService.GetMachineUUID(ctx, coremachine.Name(machineID))
	if err != nil {
		return "", errors.Trace(err)
	}
	watcher, err := u.machineService.WatchLXDProfiles(ctx, machineUUID)
	if err != nil {
		return "", errors.Trace(err)
	}
	watcherID, _, err := internal.EnsureRegisterWatcher[struct{}](ctx, u.watcherRegistry, watcher)
	return watcherID, err
}

// LXDProfileName returns the name of the lxd profile applied to the unit's
// machine for the current charm version.
func (u *LXDProfileAPIv2) LXDProfileName(ctx context.Context, args params.Entities) (params.StringResults, error) {
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
		unit, _, err := u.getLXDProfileUnitMachineV2(tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		machineTagID, err := unit.AssignedMachineId()
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		machineUUID, err := u.machineService.GetMachineUUID(ctx, coremachine.Name(machineTagID))
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		name, err := u.getOneLXDProfileName(ctx, unit, machineUUID)
		if errors.Is(err, machineerrors.NotProvisioned) {
			result.Results[i].Error = apiservererrors.ServerError(errors.NotProvisionedf("machine %q", machineTagID))
		} else if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		result.Results[i].Result = name

	}
	return result, nil
}

func (u *LXDProfileAPIv2) getOneLXDProfileName(ctx context.Context, unit LXDProfileUnitV2, machineUUID string) (string, error) {
	profileNames, err := u.machineService.AppliedLXDProfileNames(ctx, machineUUID)
	if err != nil {
		u.logger.Errorf(ctx, "unable to retrieve LXD profiles for machine %q: %v", machineUUID, err)
		return "", err
	}
	appName := unit.ApplicationName()
	return lxdprofile.MatchProfileNameByAppName(profileNames, appName)
}

// CanApplyLXDProfile returns true if
//   - this is an IAAS model,
//   - the unit is not on a manual machine,
//   - the provider type is "lxd" or it's an lxd container.
func (u *LXDProfileAPIv2) CanApplyLXDProfile(ctx context.Context, args params.Entities) (params.BoolResults, error) {
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
		machine, err := u.getLXDProfileMachineV2(tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		name, err := u.getOneCanApplyLXDProfile(machine, modelInfo.CloudType)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		result.Results[i].Result = name

	}
	return result, nil
}

func (u *LXDProfileAPIv2) getOneCanApplyLXDProfile(machine LXDProfileMachineV2, providerType string) (bool, error) {
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
func (u *LXDProfileAPIv2) LXDProfileRequired(ctx context.Context, args params.CharmURLs) (params.BoolResults, error) {
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

func (u *LXDProfileAPIv2) getOneLXDProfileRequired(ctx context.Context, curl string) (bool, error) {
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

func (u *LXDProfileAPIv2) getLXDProfileMachineV2(tag names.Tag) (LXDProfileMachineV2, error) {
	_, machine, err := u.getLXDProfileUnitMachineV2(tag)
	return machine, err
}

func (u *LXDProfileAPIv2) getLXDProfileUnitMachineV2(tag names.Tag) (LXDProfileUnitV2, LXDProfileMachineV2, error) {
	var id string
	if tag.Kind() != names.UnitTagKind {
		return nil, nil, errors.Errorf("not a unit tag")
	}
	unit, err := u.backend.Unit(tag.Id())
	if err != nil {
		return nil, nil, err
	}
	id, err = unit.AssignedMachineId()
	if err != nil {
		return nil, nil, err
	}
	machine, err := u.backend.Machine(id)
	return unit, machine, err
}
