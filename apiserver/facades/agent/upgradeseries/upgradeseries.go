// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeseries

import (
	"github.com/juju/loggo"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/state"
	"gopkg.in/juju/names.v2"
)

var logger = loggo.GetLogger("juju.apiserver.upgradeseries")

type UpgradeSeriesAPI struct {
	*common.UpgradeSeriesAPI

	st        *state.State
	auth      facade.Authorizer
	resources facade.Resources
}

func NewUpgradeSeriesAPI(st *state.State, resources facade.Resources, authorizer facade.Authorizer) (*UpgradeSeriesAPI, error) {
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
		st:               st,
		resources:        resources,
		auth:             authorizer,
		UpgradeSeriesAPI: common.NewExternalUpgradeSeriesAPI(st, resources, authorizer, accessMachine, accessUnit, logger),
	}, nil
}
