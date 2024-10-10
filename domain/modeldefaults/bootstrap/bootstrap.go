// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	"github.com/juju/juju/domain/modeldefaults"
	"github.com/juju/juju/domain/modeldefaults/service"
	"github.com/juju/juju/domain/modeldefaults/state"
	"github.com/juju/juju/internal/errors"
)

// ModelDefaultsProvider is a bootstrap helper that wraps the raw config values
// passed in during bootstrap into a model default provider interface to be used
// when persisting initial model config. Config passed to this func can be nil.
func ModelDefaultsProvider(
	controllerConfig map[string]any,
	cloudRegionConfig map[string]any,
	cloudType string,
) service.ModelDefaultsProviderFunc {
	return func(ctx context.Context) (modeldefaults.Defaults, error) {
		defaults := modeldefaults.Defaults{}

		for k, v := range state.ConfigDefaults(ctx) {
			defaults[k] = modeldefaults.DefaultAttributeValue{
				Default: v,
			}
		}

		providerDefaults, _, err := service.ProviderDefaults(
			context.Background(),
			cloudType,
			service.ProviderModelConfigGetter(),
		)
		if err != nil {
			return nil, errors.Errorf(
				"getting provider defaults for bootstrap: %w", err,
			)
		}

		for k, v := range providerDefaults {
			defaults[k] = v
		}

		for k, v := range controllerConfig {
			defaults[k] = modeldefaults.DefaultAttributeValue{
				Controller: v,
			}
		}

		for k, v := range cloudRegionConfig {
			defaults[k] = modeldefaults.DefaultAttributeValue{
				Region: v,
			}
		}

		return defaults, nil
	}
}
