// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/juju/juju/core/database"
	coremodel "github.com/juju/juju/core/model"
	modelstate "github.com/juju/juju/domain/model/state"
	"github.com/juju/juju/domain/modelconfig/service"
	"github.com/juju/juju/domain/modelconfig/state"
	"github.com/juju/juju/environs/config"
	internaldatabase "github.com/juju/juju/internal/database"
)

// SetModelConfig will remove any existing model config for the model and
// replace with the new config provided. The new config will not be hydrated
// with any model default attributes that have not been set on the config.
func SetModelConfig(
	modelID coremodel.UUID,
	attrs map[string]any,
	defaultsProvider service.ModelDefaultsProvider,
) internaldatabase.BootstrapOpt {
	return func(ctx context.Context, controller, model database.TxnRunner) error {
		if attrs == nil {
			attrs = map[string]any{}
		}
		defaults, err := defaultsProvider.ModelDefaults(ctx)
		if err != nil {
			return fmt.Errorf("getting model defaults: %w", err)
		}

		for k, v := range defaults {
			attrVal := v.ApplyStrategy(attrs[k])
			if attrVal != nil {
				attrs[k] = attrVal
			}
		}

		var m coremodel.Model
		err = controller.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
			var err error
			m, err = modelstate.Get(ctx, tx, modelID)
			return err
		})

		if err != nil {
			return fmt.Errorf("setting model %q config: %w", modelID, err)
		}

		attrs[config.UUIDKey] = m.UUID
		attrs[config.TypeKey] = m.ModelType
		attrs[config.NameKey] = m.Name

		// TODO (tlm): Currently the Juju client passes agent version to a
		// bootstrap controller via model config. Yep very very very silly.
		// This needs a bit more modelling in DQlite before to change the flow.
		// To make it more digestible of the bootstrap code we are throwing it
		// away here.
		//
		// What needs to happen:
		// - model agent version in the model database correctly.
		// - change any client code that is passing the value via config.
		// - add migration logic to get rid of agent version out of config.
		delete(attrs, config.AgentVersionKey)

		cfg, err := config.New(config.NoDefaults, attrs)
		if err != nil {
			return fmt.Errorf("constructing new model config with model defaults: %w", err)
		}

		_, err = config.ModelValidator().Validate(ctx, cfg, nil)
		if err != nil {
			return fmt.Errorf("validating model config to set for model: %w", err)
		}

		rawCfg, err := service.CoerceConfigForStorage(cfg.AllAttrs())
		if err != nil {
			return fmt.Errorf("coercing model config for storage: %w", err)
		}

		return model.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
			return state.SetModelConfig(ctx, rawCfg, tx)
		})
	}
}
