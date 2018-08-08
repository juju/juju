// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeseries

import (
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.upgradeseries")

type State interface {
	common.UpgradeSeriesBackend
}

type API struct {
	*common.UpgradeSeriesAPI

	st        State
	auth      facade.Authorizer
	resources facade.Resources
}

func NewAPI(state *state.State, resources facade.Resources, authorizer facade.Authorizer) (*API, error) {
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

	st := common.UpgradeSeriesState{state}
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
		tag, err := names.ParseMachineTag(entity.Tag)
		if err != nil {
			results[i].Error = common.ServerError(err)
			continue
		}
		if !canAccess(tag) {
			results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}

		machine, err := a.GetMachine(tag)
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
