// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/state"
)

// This file contains untested shims to let us wrap state in a sensible
// interface and avoid writing tests that depend on mongodb. If you were
// to change any part of it so that it were no longer *obviously* and
// *trivially* correct, you would be Doing It Wrong.

// TODO - CAAS(ericclaudejones): This should contain state alone, model will be
// removed once all relevant methods are moved from state to model.
type stateShim struct {
	*state.State
	*state.Model
}

// MachineSeries implements backups.Backend
func (s *stateShim) MachineSeries(id string) (string, error) {
	m, err := s.State.Machine(id)
	if err != nil {
		return "", errors.Trace(err)
	}
	return m.Series(), nil
}

// NewFacade provides the required signature for facade registration.
func NewFacade(st *state.State, resources facade.Resources, authorizer facade.Authorizer) (*API, error) {
	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewAPI(&stateShim{st, model}, resources, authorizer)
}

// ControllerTag disambiguates the ControllerTag method pending further
// refactoring to separate model functionality from state functionality.
func (s *stateShim) ControllerTag() names.ControllerTag {
	return s.State.ControllerTag()
}

// ModelTag disambiguates the ControllerTag method pending further refactoring
// to separate model functionality from state functionality.
func (s *stateShim) ModelTag() names.ModelTag {
	return s.Model.ModelTag()
}
