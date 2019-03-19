// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import "github.com/juju/juju/state"

var (
	ParseSettingsCompatible = parseSettingsCompatible
	NewStateStorage         = &newStateStorage
	GetStorageState         = getStorageState
	OpenCharmStoreRepo      = openCSRepo
)

func GetState(st *state.State) Backend {
	return stateShim{st}
}

func SetModelType(api *APIv9, modelType state.ModelType) {
	api.modelType = modelType
}
