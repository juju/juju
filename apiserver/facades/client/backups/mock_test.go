// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/state"
)

// TODO - CAAS(ericclaudejones): This should contain state alone, model will be
// removed once all relevant methods are moved from state to model.
type stateShim struct {
	*state.State
	*state.Model
}

func (s *stateShim) MachineSeries(id string) (string, error) {
	return "xenial", nil
}

func (s *stateShim) ControllerTag() names.ControllerTag {
	return s.State.ControllerTag()
}
