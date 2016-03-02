// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"github.com/juju/errors"

	"github.com/juju/juju/core/description"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

// ImportModel deserializes a model description from the bytes, transforms
// the model config based on information from the controller model, and then
// imports that as a new database model.
func ImportModel(st *state.State, bytes []byte) (*state.Model, *state.State, error) {
	model, err := description.Deserialize(bytes)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	controllerModel, err := st.ControllerModel()
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	controllerConfig, err := controllerModel.Config()
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	model.UpdateConfig(controllerValues(controllerConfig))

	if err := updateConfigFromProvider(model, controllerConfig); err != nil {
		return nil, nil, errors.Trace(err)
	}

	dbModel, dbState, err := st.Import(model)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	return dbModel, dbState, nil
}

func controllerValues(config *config.Config) map[string]interface{} {
	result := make(map[string]interface{})

	result["state-port"] = config.StatePort()
	result["api-port"] = config.APIPort()
	// We ignore the second bool param from the CACert check as if there
	// wasn't a CACert, there is no way we'd be importing a new model
	// into the controller
	result["ca-cert"], _ = config.CACert()

	return result
}

func updateConfigFromProvider(model description.Model, controllerConfig *config.Config) error {
	newConfig, err := config.New(config.NoDefaults, model.Config())
	if err != nil {
		return errors.Trace(err)
	}

	provider, err := environs.New(newConfig)
	if err != nil {
		return errors.Trace(err)
	}

	updater, ok := provider.(environs.MigrationConfigUpdater)
	if !ok {
		return nil
	}

	model.UpdateConfig(updater.MigrationConfigUpdate(controllerConfig))
	return nil
}
