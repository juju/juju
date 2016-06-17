// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/state"
)

// This file contains untested shims to let us wrap state in a sensible
// interface and avoid writing tests that depend on mongodb. If you were
// to change any part of it so that it were no longer *obviously* and
// *trivially* correct, you would be Doing It Wrong.

func init() {
	common.RegisterStandardFacade("Backups", 1, newAPI)
}

type stateShim struct {
	*state.State
}

// MachineSeries implements backups.Backend
func (s *stateShim) MachineSeries(id string) (string, error) {
	m, err := s.State.Machine(id)
	if err != nil {
		return "", errors.Trace(err)
	}
	return m.Series(), nil
}

func newAPI(st *state.State, resources *common.Resources, authorizer common.Authorizer) (*API, error) {
	return NewAPI(&stateShim{st}, resources, authorizer)
}
