// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/state"
)

var (
	GetAllUnitNames = getAllUnitNames
	QueueActions    = &queueActions
)

func NewActionAPI(st *state.State, resources facade.Resources, authorizer facade.Authorizer) (*ActionAPI, error) {
	return newActionAPI(&stateShim{st: st}, resources, authorizer)
}

func NewActionAPIForMockTest(st State, resources facade.Resources, authorizer facade.Authorizer, tagToActionReceiverFn TagToActionReceiverFunc) (*ActionAPI, error) {
	api, err := newActionAPI(st, resources, authorizer)
	if err != nil {
		return api, err
	}
	api.tagToActionReceiverFn = tagToActionReceiverFn
	return api, nil
}

func StateShimForTest(st *state.State) *stateShim {
	return &stateShim{st: st}
}
