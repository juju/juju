// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	"github.com/juju/juju/domain/modeldefaults"
	"github.com/juju/juju/domain/modeldefaults/service"
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
			attr := defaults[k]
			attr.Default = v
			defaults[k] = attr
		}

		for k, v := range controllerConfig {
			attr := defaults[k]
			attr.Controller = v
			defaults[k] = attr
		}

		for k, v := range cloudRegionConfig {
			attr := defaults[k]
			attr.Region = v
			defaults[k] = attr
		}

		return defaults, nil
	}
}
