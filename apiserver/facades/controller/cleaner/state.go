// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cleaner

import (
	"github.com/juju/errors"

	commonsecrets "github.com/juju/juju/apiserver/common/secrets"
	"github.com/juju/juju/state"
)

type StateInterface interface {
	Cleanup(secretContentDeleter state.SecretContentDeleter) error
	WatchCleanups() state.NotifyWatcher
	SecretsModel() (commonsecrets.Model, error)
}

type stateShim struct {
	*state.State
}

func (s stateShim) SecretsModel() (commonsecrets.Model, error) {
	m, err := s.State.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return commonsecrets.SecretsModel(m), nil
}

var getState = func(st *state.State) StateInterface {
	return stateShim{st}
}
