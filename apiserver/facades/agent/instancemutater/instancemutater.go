// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater

import (
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/loggo"
)

var logger = loggo.GetLogger("juju.apiserver.instancemutater")

type InstanceMutaterAPIV1 interface {
	Life(args params.Entities) (params.LifeResults, error)
	WatchUnits(args params.Entities) (params.StringsWatchResults, error)
	WatchModelMachines() (params.StringsWatchResult, error)
}

type InstanceMutaterAPI struct {
	*common.ModelMachinesWatcher
	*common.UnitsWatcher
	*common.LifeGetter

	st          InstanceMutaterState
	getAuthFunc common.GetAuthFunc
}

func NewInstanceMutaterFacade(ctx facade.Context) (*InstanceMutaterAPI, error) {
	st := &instanceMutaterShim{State: ctx.State()}
	return NewInstanceMutaterAPI(st, ctx.Resources(), ctx.Auth())
}

func NewInstanceMutaterAPI(st InstanceMutaterState, resources facade.Resources, authorizer facade.Authorizer) (*InstanceMutaterAPI, error) {
	if !authorizer.AuthMachineAgent() && !authorizer.AuthController() {
		return nil, common.ErrPerm
	}

	getAuthFunc := common.AuthFuncForMachineAgent(authorizer)
	getCanWatch := func() (common.AuthFunc, error) {
		return authorizer.AuthOwner, nil
	}
	return &InstanceMutaterAPI{
		ModelMachinesWatcher: common.NewModelMachinesWatcher(st, resources, authorizer),
		UnitsWatcher:         common.NewUnitsWatcher(st, resources, getCanWatch),
		LifeGetter:           common.NewLifeGetter(st, getAuthFunc),
		st:                   st,
		getAuthFunc:          getAuthFunc,
	}, nil
}
