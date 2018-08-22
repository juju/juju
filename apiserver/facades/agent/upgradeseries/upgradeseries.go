// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeseries

import (
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.upgradeseries")

// State exposes methods from state required by this API.
type State interface {
	common.UpgradeSeriesBackend
}

// API serves methods required by the machine agent upgrade-series worker.
type API struct {
	*common.UpgradeSeriesAPI

	st        State
	auth      facade.Authorizer
	resources facade.Resources
}

// NewAPI creates a new instance of the API server.
// It has a signature suitable for external registration.
func NewAPI(st *state.State, resources facade.Resources, authorizer facade.Authorizer) (*API, error) {
	return NewUpgradeSeriesAPI(common.UpgradeSeriesState{St: st}, resources, authorizer)
}

// NewUpgradeSeriesAPI creates a new instance of the API server using the
// dedicated state indirection.
func NewUpgradeSeriesAPI(st State, resources facade.Resources, authorizer facade.Authorizer) (*API, error) {
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
		UpgradeSeriesAPI: common.NewUpgradeSeriesAPI(st, resources, authorizer, accessMachine, accessUnit, logger),
	}, nil
}

// MachineStatus gets the current upgrade-series status of a machine.
func (a *API) MachineStatus(args params.Entities) (params.UpgradeSeriesStatusResultsNew, error) {
	result := params.UpgradeSeriesStatusResultsNew{}

	canAccess, err := a.AccessMachine()
	if err != nil {
		return result, err
	}

	results := make([]params.UpgradeSeriesStatusResultNew, len(args.Entities))
	for i, entity := range args.Entities {
		machine, err := a.authAndMachine(entity, canAccess)
		if err != nil {
			results[i].Error = common.ServerError(err)
			continue
		}
		status, err := machine.MachineUpgradeSeriesStatus()
		if err != nil {
			results[i].Error = common.ServerError(err)
			continue
		}
		results[i].Status = params.UpgradeSeriesStatus{Status: status, Entity: entity}
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
		err = machine.SetMachineUpgradeSeriesStatus(param.Status)
		if err != nil {
			results[i].Error = common.ServerError(err)
		}
	}

	result.Results = results
	return result, nil
}

// CompleteStatus starts the upgrade series completion phase for all subordinate
// units of a given machine.
func (a *API) StartUnitCompletion(args params.SetUpgradeSeriesStatusParams) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Params)),
	}
	canAccess, err := a.AccessMachine()
	if err != nil {
		return params.ErrorResults{}, err
	}
	for i, param := range args.Params {
		machine, err := a.authAndMachine(param.Entity, canAccess)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		err = machine.StartUpgradeSeriesUnitCompletion()
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
	}
	return result, nil
}

// UnitsReadyToStop returns the units running on this machine that have
// completed their upgrade-series preparation, and are ready to be stopped and
// have their unit agent services converted for the target series.
func (a *API) UnitsReadyToStop(args params.Entities) (params.EntitiesResults, error) {
	result := params.EntitiesResults{}
	//
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
			if s.Status == model.PrepareCompleted {
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
