// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeseries

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// API serves methods required by the machine agent upgrade-machine worker.
type API struct {
	*common.UpgradeSeriesAPI

	st              common.UpgradeSeriesBackend
	auth            facade.Authorizer
	resources       facade.Resources
	leadership      *common.LeadershipPinning
	logger          loggo.Logger
	historyRecorder status.StatusHistoryRecorder
}

// NewUpgradeSeriesAPI creates a new instance of the API server using the
// dedicated state indirection.
func NewUpgradeSeriesAPI(
	st common.UpgradeSeriesBackend,
	resources facade.Resources,
	authorizer facade.Authorizer,
	leadership *common.LeadershipPinning,
	logger loggo.Logger,
	historyRecorder status.StatusHistoryRecorder,
) (*API, error) {
	if !authorizer.AuthMachineAgent() {
		return nil, apiservererrors.ErrPerm
	}

	accessMachine := func() (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			return authorizer.AuthOwner(tag)
		}, nil
	}
	accessUnit := func() (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			return false
		}, nil
	}

	return &API{
		st:               st,
		resources:        resources,
		auth:             authorizer,
		leadership:       leadership,
		UpgradeSeriesAPI: common.NewUpgradeSeriesAPI(st, resources, authorizer, accessMachine, accessUnit, logger),
		historyRecorder:  historyRecorder,
	}, nil
}

// MachineStatus gets the current upgrade-machine status of a machine.
func (a *API) MachineStatus(ctx context.Context, args params.Entities) (params.UpgradeSeriesStatusResults, error) {
	result := params.UpgradeSeriesStatusResults{}

	canAccess, err := a.AccessMachine()
	if err != nil {
		return result, err
	}

	results := make([]params.UpgradeSeriesStatusResult, len(args.Entities))
	for i, entity := range args.Entities {
		machine, err := a.authAndGetMachine(ctx, entity.Tag, canAccess)
		if err != nil {
			results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		status, err := machine.UpgradeSeriesStatus()
		if err != nil {
			results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results[i].Status = status
	}

	result.Results = results
	return result, nil
}

// SetMachineStatus sets the current upgrade-machine status of a machine.
func (a *API) SetMachineStatus(ctx context.Context, args params.UpgradeSeriesStatusParams) (params.ErrorResults, error) {
	result := params.ErrorResults{}

	canAccess, err := a.AccessMachine()
	if err != nil {
		return result, err
	}

	results := make([]params.ErrorResult, len(args.Params))
	for i, param := range args.Params {
		machine, err := a.authAndGetMachine(ctx, param.Entity.Tag, canAccess)
		if err != nil {
			results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		err = machine.SetUpgradeSeriesStatus(param.Status, param.Message)
		if err != nil {
			results[i].Error = apiservererrors.ServerError(err)
		}
	}

	result.Results = results
	return result, nil
}

// CurrentSeries returns what Juju thinks the current series of the machine is.
// Note that a machine could have been upgraded out-of-band by running
// do-release-upgrade outside of the upgrade-machine workflow,
// making this value incorrect.
func (a *API) CurrentSeries(ctx context.Context, args params.Entities) (params.StringResults, error) {
	result := params.StringResults{}

	canAccess, err := a.AccessMachine()
	if err != nil {
		return result, err
	}

	results := make([]params.StringResult, len(args.Entities))
	for i, entity := range args.Entities {
		machine, err := a.authAndGetMachine(ctx, entity.Tag, canAccess)
		if err != nil {
			results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		series, err := corebase.GetSeriesFromChannel(machine.Base().OS, machine.Base().Channel)
		if err != nil {
			results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results[i].Result = series
	}

	result.Results = results
	return result, nil
}

// TargetSeries returns the series that a machine has been locked
// for upgrading to.
func (a *API) TargetSeries(ctx context.Context, args params.Entities) (params.StringResults, error) {
	result := params.StringResults{}

	canAccess, err := a.AccessMachine()
	if err != nil {
		return result, err
	}

	results := make([]params.StringResult, len(args.Entities))
	for i, entity := range args.Entities {
		machine, err := a.authAndGetMachine(ctx, entity.Tag, canAccess)
		if err != nil {
			results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		target, err := machine.UpgradeSeriesTarget()
		if err != nil {
			results[i].Error = apiservererrors.ServerError(err)
		}
		results[i].Result = target
	}

	result.Results = results
	return result, nil
}

// StartUnitCompletion starts the upgrade series completion phase for all subordinate
// units of a given machine.
func (a *API) StartUnitCompletion(ctx context.Context, args params.UpgradeSeriesStartUnitCompletionParam) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	canAccess, err := a.AccessMachine()
	if err != nil {
		return params.ErrorResults{}, err
	}
	for i, entity := range args.Entities {
		machine, err := a.authAndGetMachine(ctx, entity.Tag, canAccess)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		err = machine.StartUpgradeSeriesUnitCompletion(args.Message)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
	}
	return result, nil
}

// FinishUpgradeSeries is the last action in the upgrade workflow and is
// called after all machine and unit statuses are "completed".
// It updates the machine series to reflect the completed upgrade, then
// removes the upgrade-machine lock.
func (a *API) FinishUpgradeSeries(ctx context.Context, args params.UpdateChannelArgs) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Args)),
	}
	canAccess, err := a.AccessMachine()
	if err != nil {
		return params.ErrorResults{}, err
	}
	for i, arg := range args.Args {
		machine, err := a.authAndGetMachine(ctx, arg.Entity.Tag, canAccess)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		// Actually running "do-release-upgrade" is not required to complete a
		// series upgrade, so we compare the incoming host OS with the machine.
		// Only update if they differ, because calling UpgradeSeriesTarget
		// cascades through units and subordinates to verify series support,
		// which we might as well skip unless an update is required.
		mBase := machine.Base()
		var argBase state.Base
		if arg.Channel != "" {
			argBase = state.Base{OS: mBase.OS, Channel: arg.Channel}.Normalise()
		}
		if argBase.String() == mBase.String() {
			a.logger.Debugf("%q base is unchanged from %q", arg.Entity.Tag, mBase.DisplayString())
		} else {
			if err := machine.UpdateMachineSeries(argBase); err != nil {
				result.Results[i].Error = apiservererrors.ServerError(err)
				continue
			}
		}

		err = machine.RemoveUpgradeSeriesLock()
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
	}
	return result, nil
}

// UnitsPrepared returns the units running on this machine that have completed
// their upgrade-machine preparation, and are ready to be stopped and have their
// unit agent services converted for the target series.
func (a *API) UnitsPrepared(ctx context.Context, args params.Entities) (params.EntitiesResults, error) {
	result, err := a.unitsInState(ctx, args, model.UpgradeSeriesPrepareCompleted)
	return result, errors.Trace(err)
}

// UnitsCompleted returns the units running on this machine that have completed
// the upgrade-machine workflow and are in their normal running state.
func (a *API) UnitsCompleted(ctx context.Context, args params.Entities) (params.EntitiesResults, error) {
	result, err := a.unitsInState(ctx, args, model.UpgradeSeriesCompleted)
	return result, errors.Trace(err)
}

func (a *API) unitsInState(ctx context.Context, args params.Entities, status model.UpgradeSeriesStatus) (params.EntitiesResults, error) {
	result := params.EntitiesResults{}

	canAccess, err := a.AccessMachine()
	if err != nil {
		return result, err
	}

	results := make([]params.EntitiesResult, len(args.Entities))
	for i, entity := range args.Entities {
		machine, err := a.authAndGetMachine(ctx, entity.Tag, canAccess)
		if err != nil {
			results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		statuses, err := machine.UpgradeSeriesUnitStatuses()
		if err != nil {
			results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		var entities []params.Entity
		for id, s := range statuses {
			if s.Status == status {
				entities = append(entities, params.Entity{Tag: names.NewUnitTag(id).String()})
			}
		}
		results[i].Entities = entities
	}

	result.Results = results
	return result, nil
}

func (a *API) authAndGetMachine(ctx context.Context, entityTag string, canAccess common.AuthFunc) (common.UpgradeSeriesMachine, error) {
	tag, err := names.ParseMachineTag(entityTag)
	if err != nil {
		return nil, err
	}
	if !canAccess(tag) {
		return nil, apiservererrors.ErrPerm
	}
	return a.GetMachine(ctx, tag)
}

// PinnedLeadership returns all pinned applications and the entities that
// require their pinned behaviour, for leadership in the current model.
func (a *API) PinnedLeadership(ctx context.Context) (params.PinnedLeadershipResult, error) {
	return a.leadership.PinnedLeadership(ctx)
}

// PinMachineApplications pins leadership for applications represented by units
// running on the auth'd machine.
func (a *API) PinMachineApplications(ctx context.Context) (params.PinApplicationsResults, error) {
	return a.leadership.PinApplicationLeaders(ctx)
}

// UnpinMachineApplications unpins leadership for applications represented by
// units running on the auth'd machine.
func (a *API) UnpinMachineApplications(ctx context.Context) (params.PinApplicationsResults, error) {
	return a.leadership.UnpinApplicationLeaders(ctx)
}

// SetInstanceStatus sets the status of the machine.
func (a *API) SetInstanceStatus(ctx context.Context, args params.SetStatus) (params.ErrorResults, error) {
	result := params.ErrorResults{}

	canAccess, err := a.AccessMachine()
	if err != nil {
		return result, err
	}

	results := make([]params.ErrorResult, len(args.Entities))
	for i, entity := range args.Entities {
		machine, err := a.authAndGetMachine(ctx, entity.Tag, canAccess)
		if err != nil {
			results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		if err := machine.SetInstanceStatus(status.StatusInfo{
			Status:  status.Status(entity.Status),
			Message: entity.Info,
		}, a.historyRecorder); err != nil {
			results[i].Error = apiservererrors.ServerError(err)
		}
	}

	result.Results = results
	return result, nil
}
