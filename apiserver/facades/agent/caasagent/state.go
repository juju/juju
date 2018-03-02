// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasagent

import (
	"github.com/juju/juju/state"
	"gopkg.in/juju/names.v2"
)

// CAASAgentState provides the subset of global state
// required by the CAAS agent facade.
type CAASAgentState interface {
	Model() (Model, error)
}

// Model provides the subset of CAAS model state required
// by the CAAS agent facade.
type Model interface {
	Name() string
	UUID() string
	Type() state.ModelType
	Owner() names.UserTag
	ModelTag() names.ModelTag
}

type stateShim struct {
	*state.State
}

func (st stateShim) Model() (Model, error) {
	return st.State.Model()
}
