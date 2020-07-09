// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeseries

import (
	"github.com/juju/errors"
	"github.com/juju/juju/core/status"
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

	st         common.UpgradeSeriesBackend
	auth       facade.Authorizer
	resources  facade.Resources
	leadership *common.LeadershipPinning
}

// NewAPI creates a new instance of the API with the given context
func NewAPI(ctx facade.Context) (*API, error) {
	leadership, err := common.NewLeadershipPinningFromContext(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewUpgradeSeriesAPI(common.UpgradeSeriesState{St: ctx.State()}, ctx.Resources(), ctx.Auth(), leadership)
}

// NewUpgradeSeriesAPI creates a new instance of the API server using the
// dedicated state indirection.
func NewUpgradeSeriesAPI(
	st common.UpgradeSeriesBackend,
	resources facade.Resources,
	authorizer facade.Authorizer,
	leadership *common.LeadershipPinning,
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
		st:               st,
		resources:        resources,
		auth:             authorizer,
		leadership:       leadership,
		UpgradeSeriesAPI: common.NewUpgradeSeriesAPI(st, resources, authorizer, accessMachine, accessUnit, logger),
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
		machine, err := a.authAndGetMachine(entity.Tag, canAccess)
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
		machine, err := a.authAndGetMachine(param.Entity.Tag, canAccess)
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

// CurrentSeries returns what Juju thinks the current series of the machine is.
// Note that a machine could have been upgraded out-of-band by running
// do-release-upgrade outside of the upgrade-series workflow,
// making this value incorrect.
func (a *API) CurrentSeries(args params.Entities) (params.StringResults, error) {
	result := params.StringResults{}

	canAccess, err := a.AccessMachine()
	if err != nil {
		return result, err
	}

	results := make([]params.StringResult, len(args.Entities))
	for i, entity := range args.Entities {
		machine, err := a.authAndGetMachine(entity.Tag, canAccess)
		if err != nil {
			results[i].Error = common.ServerError(err)
			continue
		}
		results[i].Result = machine.Series()
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
		machine, err := a.authAndGetMachine(entity.Tag, canAccess)
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
		machine, err := a.authAndGetMachine(entity.Tag, canAccess)
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
		machine, err := a.authAndGetMachine(arg.Entity.Tag, canAccess)
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
		machine, err := a.authAndGetMachine(entity.Tag, canAccess)
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

func (a *API) authAndGetMachine(entityTag string, canAccess common.AuthFunc) (common.UpgradeSeriesMachine, error) {
	tag, err := names.ParseMachineTag(entityTag)
	if err != nil {
		return nil, err
	}
	if !canAccess(tag) {
		return nil, common.ErrPerm
	}
	return a.GetMachine(tag)
}

// PinnedLeadership returns all pinned applications and the entities that
// require their pinned behaviour, for leadership in the current model.
func (a *API) PinnedLeadership() (params.PinnedLeadershipResult, error) {
	return a.leadership.PinnedLeadership()
}

// PinMachineApplications pins leadership for applications represented by units
// running on the auth'd machine.
func (a *API) PinMachineApplications() (params.PinApplicationsResults, error) {
	return a.leadership.PinApplicationLeaders()
}

// UnpinMachineApplications unpins leadership for applications represented by
// units running on the auth'd machine.
func (a *API) UnpinMachineApplications() (params.PinApplicationsResults, error) {
	return a.leadership.UnpinApplicationLeaders()
}

// SetInstanceStatus sets the status of the machine.
func (a *API) SetInstanceStatus(args params.SetStatus) (params.ErrorResults, error) {
	result := params.ErrorResults{}

	canAccess, err := a.AccessMachine()
	if err != nil {
		return result, err
	}

	results := make([]params.ErrorResult, len(args.Entities))
	for i, entity := range args.Entities {
		machine, err := a.authAndGetMachine(entity.Tag, canAccess)
		if err != nil {
			results[i].Error = common.ServerError(err)
			continue
		}

		if err := machine.SetInstanceStatus(status.StatusInfo{
			Status:  status.Status(entity.Status),
			Message: entity.Info,
		}); err != nil {
			results[i].Error = common.ServerError(err)
		}
	}

	result.Results = results
	return result, nil
}

// APIv1 provides the upgrade-series API facade for version 1.
type APIv1 struct {
	*APIv2
}

// CurrentSeries was not available on version 1 of the API.
func (api *APIv1) CurrentSeries(_, _ struct{}) {}

// APIv2 provides the upgrade-series API facade for version 2.
type APIv2 struct {
	*API
}

// SetStatus was not available on version 2 of the API.
func (api *APIv2) SetStatus(_, _ struct{}) {}

// NewAPIv1 is a wrapper that creates a V1 upgrade-series API.
func NewAPIv1(ctx facade.Context) (*APIv1, error) {
	api, err := NewAPIv2(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv1{api}, nil
}

// NewAPIv2 is a wrapper that creates a V2 upgrade-series API.
func NewAPIv2(ctx facade.Context) (*APIv2, error) {
	api, err := NewAPI(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv2{api}, nil
}
