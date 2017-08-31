// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelconfig

import (
	names "gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

// Backend contains the state.State methods used in this package,
// allowing stubs to be created for testing.
type Backend interface {
	common.BlockGetter
	ControllerTag() names.ControllerTag
	ModelTag() names.ModelTag
	ModelConfigValues() (config.ConfigValues, error)
	UpdateModelConfig(map[string]interface{}, []string, ...state.ValidateConfigFunc) error
	SetSLA(level, owner string, credentials []byte) error
	SLALevel() (string, error)
}

type stateShim struct {
	*state.State
}

func (st stateShim) ModelTag() names.ModelTag {
	m, err := st.State.Model()
	if err != nil {
		return names.NewModelTag(st.State.ModelUUID())
	}

	return m.ModelTag()
}

// NewStateBackend creates a backend for the facade to use.
func NewStateBackend(st *state.State) Backend {
	return stateShim{st}
}
