// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/state"
)

// TODO - CAAS(ericclaudejones): This should contain state alone, model will be
// removed once all relevant methods are moved from state to model.
type stateShim struct {
	*state.State
	*state.Model

	isController *bool
}

func (s *stateShim) IsController() bool {
	if s.isController == nil {
		return s.State.IsController()
	}
	return *s.isController
}

func (s *stateShim) MachineSeries(id string) (string, error) {
	return "xenial", nil
}

func (s *stateShim) ControllerTag() names.ControllerTag {
	return s.State.ControllerTag()
}

func (s *stateShim) ModelType() state.ModelType {
	return s.Model.Type()
}
