// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/state"
)

func NewActionAPI(st *state.State, resources facade.Resources, authorizer facade.Authorizer) (*ActionAPI, error) {
	return newActionAPI(&stateShim{st: st}, resources, authorizer)
}
