// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialmanager

import (
	"github.com/juju/juju/apiserver/common/credentialcommon"
	"github.com/juju/juju/state"
)

type stateShim struct {
	*state.State
}

func newStateShim(st *state.State) credentialcommon.StateBackend {
	return &stateShim{st}
}
