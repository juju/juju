// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeseries

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/model"
)

var logger = loggo.GetLogger("juju.apiserver.upgradeseries")

// API serves methods required by the machine agent upgrade-series worker.
type API struct {
	*common.UpgradeSeriesAPI
	common.LeadershipPinningAPI

	st        common.UpgradeSeriesBackend
	auth      facade.Authorizer
	resources facade.Resources
}

// NewAPI creates a new instance of the API with the given context
func NewAPI(ctx facade.Context) (*API, error) {
	leadership, err := common.NewLeadershipPinningFacade(ctx)
	if err != nil {
		if errors.IsNotImplemented(errors.Cause(err)) {
			leadership = disabledLeadershipPinningFacade{}
		} else {
			return nil, errors.Trace(err)
		}
	}
	return NewUpgradeSeriesAPI(common.UpgradeSeriesState{St: ctx.State()}, ctx.Resources(), ctx.Auth(), leadership)
}

// NewUpgradeSeriesAPI creates a new instance of the API server using the
// dedicated state indirection.
func NewUpgradeSeriesAPI(
	st common.UpgradeSeriesBackend,
	resources facade.Resources,
	authorizer facade.Authorizer,
	leadership common.LeadershipPinningAPI,
) (*API, error) {
	if !authorizer.AuthMachineAgent() {
		return nil, common.ErrPerm
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
		st:                   st,
		resources:            resources,
		auth:                 authorizer,
		UpgradeSeriesAPI:     common.NewUpgradeSeriesAPI(st, resources, authorizer, accessMachine, accessUnit, logger),
		LeadershipPinningAPI: leadership,
	}, nil
}

// MachineStatus gets the current upgrade-series status of a machine.
func (a *API) MachineStatus(args params.Entities) (params.UpgradeSeriesStatusResults, error) {
	result := params.UpgradeSeriesStatusResults{}

	canAccess, err := a.AccessMachine()
	if err != nil {
		return result, err
	}

	results := make([]params.UpgradeSeriesStatusResult, len(args.Entities))
	for i, entity := range args.Entities {
		machine, err := a.authAndMachine(entity, canAccess)
		if err != nil {
			results[i].Error = common.ServerError(err)
			continue
		}
		status, err := machine.UpgradeSeriesStatus()
		if err != nil {
			results[i].Error = common.ServerError(err)
			continue
		}
		results[i].Status = status
	}

	result.Results = results
	return result, nil
}

// SetMachineStatus sets the current upgrade-series status of a machine.
func (a *API) SetMachineStatus(args params.UpgradeSeriesStatusParams) (params.ErrorResults, error) {
	result := params.ErrorResults{}

	canAccess, err := a.AccessMachine()
	if err != nil {
		return result, err
	}

	results := make([]params.ErrorResult, len(args.Params))
	for i, param := range args.Params {
		machine, err := a.authAndMachine(param.Entity, canAccess)
		if err != nil {
			results[i].Error = common.ServerError(err)
			continue
		}
		err = machine.SetUpgradeSeriesStatus(param.Status, param.Message)
		if err != nil {
			results[i].Error = common.ServerError(err)
		}
	}

	result.Results = results
	return result, nil
}

// TargetSeries returns the series that a machine has been locked
// for upgrading to.
func (a *API) TargetSeries(args params.Entities) (params.StringResults, error) {
	result := params.StringResults{}

	canAccess, err := a.AccessMachine()
	if err != nil {
		return result, err
	}

	results := make([]params.StringResult, len(args.Entities))
	for i, entity := range args.Entities {
		machine, err := a.authAndMachine(entity, canAccess)
		if err != nil {
			results[i].Error = common.ServerError(err)
			continue
		}
		target, err := machine.UpgradeSeriesTarget()
		if err != nil {
			results[i].Error = common.ServerError(err)
		}
		results[i].Result = target
	}

	result.Results = results
	return result, nil
}

// StartUnitCompletion starts the upgrade series completion phase for all subordinate
// units of a given machine.
func (a *API) StartUnitCompletion(args params.UpgradeSeriesStartUnitCompletionParam) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	canAccess, err := a.AccessMachine()
	if err != nil {
		return params.ErrorResults{}, err
	}
	for i, entity := range args.Entities {
		machine, err := a.authAndMachine(entity, canAccess)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		err = machine.StartUpgradeSeriesUnitCompletion(args.Message)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
	}
	return result, nil
}

// FinishUpgradeSeries is the last action in the upgrade workflow and is
// called after all machine and unit statuses are "completed".
// It updates the machine series to reflect the completed upgrade, then
// removes the upgrade-series lock.
func (a *API) FinishUpgradeSeries(args params.UpdateSeriesArgs) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Args)),
	}
	canAccess, err := a.AccessMachine()
	if err != nil {
		return params.ErrorResults{}, err
	}
	for i, arg := range args.Args {
		machine, err := a.authAndMachine(arg.Entity, canAccess)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}

		// Actually running "do-release-upgrade" is not required to complete a
		// series upgrade, so we compare the incoming host OS with the machine.
		// Only update if they differ, because calling UpgradeSeriesTarget
		// cascades through units and subordinates to verify series support,
		// which we might as well skip unless an update is required.
		ms := machine.Series()
		if arg.Series == ms {
			logger.Debugf("%q series is unchanged from %q", arg.Entity.Tag, ms)
		} else {
			if err := machine.UpdateMachineSeries(arg.Series, true); err != nil {
				result.Results[i].Error = common.ServerError(err)
				continue
			}
		}

		err = machine.RemoveUpgradeSeriesLock()
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
	}
	return result, nil
}

// UnitsPrepared returns the units running on this machine that have completed
// their upgrade-series preparation, and are ready to be stopped and have their
// unit agent services converted for the target series.
func (a *API) UnitsPrepared(args params.Entities) (params.EntitiesResults, error) {
	result, err := a.unitsInState(args, model.UpgradeSeriesPrepareCompleted)
	return result, errors.Trace(err)
}

// UnitsCompleted returns the units running on this machine that have completed
// the upgrade-series workflow and are in their normal running state.
func (a *API) UnitsCompleted(args params.Entities) (params.EntitiesResults, error) {
	result, err := a.unitsInState(args, model.UpgradeSeriesCompleted)
	return result, errors.Trace(err)
}

func (a *API) unitsInState(args params.Entities, status model.UpgradeSeriesStatus) (params.EntitiesResults, error) {
	result := params.EntitiesResults{}

	canAccess, err := a.AccessMachine()
	if err != nil {
		return result, err
	}

	results := make([]params.EntitiesResult, len(args.Entities))
	for i, entity := range args.Entities {
		machine, err := a.authAndMachine(entity, canAccess)
		if err != nil {
			results[i].Error = common.ServerError(err)
			continue
		}

		statuses, err := machine.UpgradeSeriesUnitStatuses()
		if err != nil {
			results[i].Error = common.ServerError(err)
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

func (a *API) authAndMachine(e params.Entity, canAccess common.AuthFunc) (common.UpgradeSeriesMachine, error) {
	tag, err := names.ParseMachineTag(e.Tag)
	if err != nil {
		return nil, err
	}
	if !canAccess(tag) {
		return nil, common.ErrPerm
	}
	return a.GetMachine(tag)
}

// disabledLeadershipPinningFacade implements the LeadershipPinningAPI, but
// provides a no-operation for pinning operations
type disabledLeadershipPinningFacade struct{}

// PinMachineApplications implements common.LeadershipPinningAPI
func (disabledLeadershipPinningFacade) PinMachineApplications() (params.PinApplicationsResults, error) {
	return params.PinApplicationsResults{}, errors.NotImplementedf(
		"unable to get leadership pinner; pinning is not available with the legacy lease manager")
}

// UnpinMachineApplications implements common.LeadershipPinningAPI
func (disabledLeadershipPinningFacade) UnpinMachineApplications() (params.PinApplicationsResults, error) {
	return params.PinApplicationsResults{}, nil
}

// PinnedLeadership implements common.LeadershipPinningAPI
func (disabledLeadershipPinningFacade) PinnedLeadership() (params.PinnedLeadershipResult, error) {
	return params.PinnedLeadershipResult{}, nil
}
