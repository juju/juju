// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"github.com/juju/errors"

	"github.com/juju/juju/environs/config"
)

// EnvironConfigGetter exposes a model configuration to its clients.
type EnvironConfigGetter interface {
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
	env, err := newEnviron(envcfg)
	return env, errors.Trace(err)
}
