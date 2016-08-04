// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelconfig

import (
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

// Backend contains the state.State methods used in this package,
// allowing stubs to be created for testing.
type Backend interface {
	common.BlockGetter
	ModelConfigValues() (config.ConfigValues, error)
	ModelConfigDefaultValues() (config.ConfigValues, error)
	UpdateModelConfigDefaultValues(map[string]interface{}, []string) error
	UpdateModelConfig(map[string]interface{}, []string, state.ValidateConfigFunc) error
}

type stateShim struct {
	*state.State
}

// NewStateBackend creates a backend for the facade to use.
func NewStateBackend(st *state.State) Backend {
	return stateShim{st}
}
