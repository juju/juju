// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/juju/state"
)

var (
	ParseSettingsCompatible = parseSettingsCompatible
	GetStorageState         = getStorageState
	ValidateSecretConfig    = validateSecretConfig
)

func GetState(st *state.State) Backend {
	return stateShim{st}
}

func GetModel(m *state.Model) Model {
	return modelShim{m}
}
