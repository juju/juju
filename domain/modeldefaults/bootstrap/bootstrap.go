// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	modelconfigservice "github.com/juju/juju/domain/modelconfig/service"
	"github.com/juju/juju/domain/modeldefaults"
	"github.com/juju/juju/domain/modeldefaults/service"
	"github.com/juju/juju/domain/modeldefaults/state"
	internaldatabase "github.com/juju/juju/internal/database"
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

		modelConfigProviderGetter := service.ProviderModelConfigGetter()
		configProvider, err := modelConfigProviderGetter(cloudType)
		if errors.Is(err, coreerrors.NotFound) {
			return nil, errors.Errorf(
				"getting model config provider, provider for cloud type %q does not exist",
				cloudType,
			)
		} else if err != nil && !errors.Is(err, coreerrors.NotSupported) {
			return nil, errors.Errorf(
				"getting model config provider for cloud type %q: %w",
				cloudType, err,
			)
		}

		var providerDefaults map[string]any
		if configProvider != nil {
			providerDefaults, err = service.ProviderDefaults(
				context.Background(),
				cloudType,
				service.ProviderModelConfigGetter(),
			)
		}
		if err != nil {
			return nil, errors.Errorf(
				"getting provider defaults for bootstrap: %w", err,
			)
		}

		for k, v := range providerDefaults {
			defaults[k] = modeldefaults.DefaultAttributeValue{
				Default: v,
			}
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

// SetCloudDefaults is responsible for setting a previously inserted cloud's
// default config values that will be used as part of the default values
// supplied to a model's config. If no cloud exists for the specified name an
// error satisfying [github.com/juju/juju/domain/cloud/errors.NotFound] will be
// returned.
func SetCloudDefaults(
	cloudName string,
	defaults map[string]any,
) internaldatabase.BootstrapOpt {
	return func(ctx context.Context, controller, _ database.TxnRunner) error {
		strDefaults, err := modelconfigservice.CoerceConfigForStorage(defaults)
		if err != nil {
			return errors.Errorf("coercing cloud %q default values for storage: %w", cloudName, err)
		}

		err = controller.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			return state.SetCloudDefaults(ctx, tx, cloudName, strDefaults)
		})

		if err != nil {
			return errors.Errorf("setting cloud %q bootstrap defaults: %w", cloudName, err)
		}
		return nil
	}
}
