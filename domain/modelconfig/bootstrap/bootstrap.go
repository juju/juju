// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/database"
	coremodel "github.com/juju/juju/core/model"
	statecontroller "github.com/juju/juju/domain/model/state/controller"
	"github.com/juju/juju/domain/modelconfig/service"
	"github.com/juju/juju/environs/config"
	internaldatabase "github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
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
			return errors.Errorf("getting model defaults: %w", err)
		}

		for k, v := range defaults {
			attrVal := v.ApplyStrategy(attrs[k])
			if attrVal != nil {
				attrs[k] = attrVal
			}
		}

		var m coremodel.Model
		err = controller.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			var err error
			m, err = statecontroller.GetModel(ctx, tx, modelID)
			return err
		})

		if err != nil {
			return errors.Errorf("setting model %q config: %w", modelID, err)
		}

		attrs[config.UUIDKey] = m.UUID
		attrs[config.TypeKey] = m.ModelType
		attrs[config.NameKey] = m.Name

		// TODO(tlm): Currently the Juju client passes agent version and stream
		// to a bootstrapped controller via model config. We want to move away
		// from this pattern over time but until the client can be refactored we
		// remove the values from model config as they get set as first class
		// values in the model database.
		//
		// What needs to happen:
		// - change any client code that is passing the value via config.
		// - add migration logic to get rid of agent version and stream out of
		// config.
		delete(attrs, config.AgentVersionKey)
		delete(attrs, config.AgentStreamKey)

		cfg, err := config.New(config.NoDefaults, attrs)
		if err != nil {
			return errors.Errorf("constructing new model config with model defaults: %w", err)
		}

		_, err = config.ModelValidator().Validate(ctx, cfg, nil)
		if err != nil {
			return errors.Errorf("validating model config to set for model: %w", err)
		}

		insert, err := service.CoerceConfigForStorage(cfg.AllAttrs())
		if err != nil {
			return errors.Errorf("coercing model config for storage: %w", err)
		}

		insertQuery := `INSERT INTO model_config (*) VALUES ($dbKeyValue.*)`
		insertStmt, err := sqlair.Prepare(insertQuery, dbKeyValue{})
		if err != nil {
			return errors.Errorf("preparing insert query: %w", err)
		}

		return model.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			insertKV := make([]dbKeyValue, 0, len(insert))
			for k, v := range insert {
				insertKV = append(insertKV, dbKeyValue{Key: k, Value: v})
			}
			if err := tx.Query(ctx, insertStmt, insertKV).Run(); err != nil {
				return errors.Errorf("inserting model config values: %w", err)
			}
			return nil
		})
	}
}

type dbKeyValue struct {
	Key   string `db:"key"`
	Value string `db:"value"`
}
