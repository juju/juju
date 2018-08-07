// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeseries

import (
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
)

var logger = loggo.GetLogger("juju.apiserver.upgradeseries")

type State interface {
	common.UpgradeSeriesBackend
}

type UpgradeSeriesAPI struct {
	*common.UpgradeSeriesAPI

	st        State
	auth      facade.Authorizer
	resources facade.Resources
}

func NewUpgradeSeriesAPI(
	st State, resources facade.Resources, authorizer facade.Authorizer,
) (*UpgradeSeriesAPI, error) {
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

	return &UpgradeSeriesAPI{
		st:        st,
		resources: resources,
		auth:      authorizer,
		UpgradeSeriesAPI: common.NewUpgradeSeriesAPI(
			st, resources, authorizer, accessMachine, accessUnit, logger,
		),
	}, nil
}

// MachineStatus gets the current upgrade-series status of a machine.
func (u *UpgradeSeriesAPI) MachineStatus(args params.Entities) (params.UpgradeSeriesStatusResultsNew, error) {
	result := params.UpgradeSeriesStatusResultsNew{}

	canAccess, err := u.AccessMachine()
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

		machine, err := u.GetMachine(tag)
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
