// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"github.com/juju/errors"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
)

// ConfigGetter exposes a controller and model configuration to its clients.
type ConfigGetter interface {
	ModelConfig() (*config.Config, error)
	ControllerConfig() (controller.Config, error)
}

// NewEnvironFunc is the type of a function that, given a model config,
// returns an Environ. This will typically be environs.New.
type NewEnvironFunc func(*config.Config) (environs.Environ, error)

// GetEnviron returns the environs.Environ ("provider") associated
// with the model.
func GetEnviron(st ConfigGetter, newEnviron NewEnvironFunc) (environs.Environ, error) {
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
	envcfg, err = envcfg.Apply(controllerCfg)
	if err != nil {
		return nil, errors.Trace(err)
	}
	env, err := newEnviron(envcfg)
	return env, errors.Trace(err)
}
