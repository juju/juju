// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	"github.com/juju/juju/domain/modeldefaults"
	"github.com/juju/juju/domain/modeldefaults/service"
	"github.com/juju/juju/environs/config"
)

// ModelDefaultsProvider is a bootstrap helper that wraps the raw config values
// passed in during bootstrap into a model default provider interface to be used
// when persisting initial model config. Config passed to this func can be nil.
func ModelDefaultsProvider(
	configDefaults map[string]any,
	controllerConfig map[string]any,
	cloudRegionConfig map[string]any,
) service.ModelDefaultsProviderFunc {
	return func(ctx context.Context) (modeldefaults.Defaults, error) {
		defaults := modeldefaults.Defaults{}

		for k, v := range configDefaults {
			defaults[k] = modeldefaults.DefaultAttributeValue{
				Source: config.JujuDefaultSource,
				V:      v,
			}
		}

		for k, v := range controllerConfig {
			defaults[k] = modeldefaults.DefaultAttributeValue{
				Source: config.JujuControllerSource,
				V:      v,
			}
		}

		for k, v := range cloudRegionConfig {
			defaults[k] = modeldefaults.DefaultAttributeValue{
				Source: config.JujuRegionSource,
				V:      v,
			}
		}

		return defaults, nil
	}
}
