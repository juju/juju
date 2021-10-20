// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/juju/core/assumes"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
)

var (
	ParseSettingsCompatible = parseSettingsCompatible
	NewStateStorage         = &newStateStorage
	GetStorageState         = getStorageState
)

func GetState(st *state.State) Backend {
	return stateShim{st}
}

func GetModel(m *state.Model) Model {
	return modelShim{m}
}

func SetModelType(api *APIv13, modelType state.ModelType) {
	api.modelType = modelType
}

func MockSupportedFeatures(fs assumes.FeatureSet) {
	supportedFeaturesGetter = func(stateenvirons.Model, environs.NewEnvironFunc) (assumes.FeatureSet, error) {
		return fs, nil
	}
}

func ResetSupportedFeaturesGetter() {
	supportedFeaturesGetter = stateenvirons.SupportedFeatures
}
