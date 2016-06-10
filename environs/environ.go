// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"github.com/juju/errors"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/environs/config"
)

// EnvironConfigGetter exposes a controller and model configuration to its clients.
// TODO(wallyworld) - we want to avoid the need to get controller config in future
// since the controller uuid and api port can be added to StartInstanceParams.
type EnvironConfigGetter interface {
	ControllerConfig() (controller.Config, error)
	ModelConfig() (*config.Config, error)
}

// NewEnvironFunc is the type of a function that, given a model config,
// returns an Environ. This will typically be environs.New.
type NewEnvironFunc func(*config.Config) (Environ, error)

// GetEnviron returns the environs.Environ ("provider") associated
// with the model.
func GetEnviron(st EnvironConfigGetter, newEnviron NewEnvironFunc) (Environ, error) {
	envcfg, err := st.ModelConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}
	// Add in the controller config as currently environs
	// use a single config bucket for everything.
	controllerCfg, err := st.ControllerConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}
	envcfg, err = envcfg.Apply(map[string]interface{}{
		controller.ApiPort:           controllerCfg.APIPort(),
		controller.ControllerUUIDKey: controllerCfg.ControllerUUID(),
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	env, err := newEnviron(envcfg)
	return env, errors.Trace(err)
}
