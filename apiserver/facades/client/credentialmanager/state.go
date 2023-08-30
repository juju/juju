// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialmanager

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common/credentialcommon"
	"github.com/juju/juju/state"
)

type stateShim struct {
	*state.State
	*state.Model
}

func newStateShim(st *state.State) (credentialcommon.StateBackend, error) {
	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &stateShim{State: st, Model: m}, nil
}
