// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialmanager

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common/credentialcommon"
	"github.com/juju/juju/state"
)

type stateShim struct {
	*state.State
	*state.Model
}

func (s stateShim) CloudCredentialTag() (names.CloudCredentialTag, bool, error) {
	credTag, exists := s.Model.CloudCredentialTag()
	return credTag, exists, nil
}

func newStateShim(st *state.State) (credentialcommon.StateBackend, error) {
	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &stateShim{State: st, Model: m}, nil
}
