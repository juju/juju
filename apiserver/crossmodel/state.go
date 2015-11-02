// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

type crossmodelAccess interface {
	Offer(one params.CrossModelOffer) error
}

var getState = func(st *state.State) crossmodelAccess {
	return stateShim{st, make(map[string]params.CrossModelOffer)}
}

type stateShim struct {
	*state.State

	temp map[string]params.CrossModelOffer
}

// Offer prepares service endpoints for consumption.
func (s stateShim) Offer(one params.CrossModelOffer) error {
	s.temp[one.Service] = one
	return nil
}
