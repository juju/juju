// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"github.com/juju/errors"

	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
)

// EnvironConfigGetter exposes a model configuration to its clients.
type EnvironConfigGetter interface {
	ModelConfig() (*config.Config, error)
	CloudSpec() (environscloudspec.CloudSpec, error)
}

// NewEnvironFunc is the type of a function that, given a model config,
// returns an Environ. This will typically be environs.New.
type NewEnvironFunc func(OpenParams) (Environ, error)

// GetEnviron returns the environs.Environ ("provider") associated
// with the model.
func GetEnviron(st EnvironConfigGetter, newEnviron NewEnvironFunc) (Environ, error) {
	env, _, err := GetEnvironAndCloud(st, newEnviron)
	return env, err
}

// GetEnvironAndCloud returns the environs.Environ ("provider") and cloud associated
// with the model.
func GetEnvironAndCloud(st EnvironConfigGetter, newEnviron NewEnvironFunc) (Environ, *environscloudspec.CloudSpec, error) {
	modelConfig, err := st.ModelConfig()
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	cloudSpec, err := st.CloudSpec()
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	env, err := newEnviron(OpenParams{
		Cloud:  cloudSpec,
		Config: modelConfig,
	})
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	return env, &cloudSpec, nil
}
