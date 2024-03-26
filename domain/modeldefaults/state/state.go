// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/core/database"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
)

// State represents a type for interacting with the underlying model defaults
// state.
type State struct {
	*domain.StateBase
}

// ConfigDefaults returns the default configuration values set in Juju.
func (s *State) ConfigDefaults(_ context.Context) map[string]any {
	return config.ConfigDefaults()
}

// ModelCloudDefaults returns the defaults associated with the model's cloud. If
// no defaults are found then an empty map will be returned with a nil error.
func (s *State) ModelCloudDefaults(
	ctx context.Context,
	uuid coremodel.UUID,
) (map[string]string, error) {
	rval := make(map[string]string)

	db, err := s.DB()
	if err != nil {
		return rval, errors.Trace(err)
	}

	cloudDefaultsStmt := `
SELECT cloud_defaults.key,
       cloud_defaults.value
FROM cloud_defaults
INNER JOIN cloud
ON cloud.uuid = cloud_defaults.cloud_uuid
INNER JOIN model m
ON m.cloud_uuid = cloud.uuid
WHERE m.uuid = ?
`

	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, cloudDefaultsStmt, uuid)
		if err != nil {
			return fmt.Errorf("fetching cloud defaults for model %q: %w", uuid, err)
		}
		defer rows.Close()

		var (
			key, val string
		)
		for rows.Next() {
			if err := rows.Scan(&key, &val); err != nil {
				return fmt.Errorf("reading cloud defaults for model %q: %w", uuid, err)
			}
			rval[key] = val
		}
		return rows.Err()
	})

	if err != nil {
		return nil, errors.Trace(err)
	}
	return rval, nil
}

// ModelCloudRegionDefaults returns the defaults associated with the model's set
// cloud region. If no defaults are found then an empty map will be returned
// with nil error.
func (s *State) ModelCloudRegionDefaults(
	ctx context.Context,
	uuid coremodel.UUID,
) (map[string]string, error) {
	rval := make(map[string]string)

	db, err := s.DB()
	if err != nil {
		return rval, errors.Trace(err)
	}

	cloudDefaultsStmt := `
SELECT cloud_region_defaults.key,
       cloud_region_defaults.value
FROM cloud_region_defaults
INNER JOIN cloud_region
ON cloud_region.uuid = cloud_region_defaults.region_uuid
INNER JOIN model m
ON m.cloud_region_uuid = cloud_region.uuid
WHERE m.uuid = ?
`

	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, cloudDefaultsStmt, uuid)
		if err != nil {
			return fmt.Errorf("fetching cloud region defaults for model %q: %w", uuid, err)
		}
		defer rows.Close()

		var (
			key, val string
		)
		for rows.Next() {
			if err := rows.Scan(&key, &val); err != nil {
				return fmt.Errorf("reading cloud region defaults for model %q: %w", uuid)
			}
			rval[key] = val
		}
		return rows.Err()
	})

	if err != nil {
		return nil, err
	}
	return rval, nil
}

// ModelMetadataDefaults is responsible for returning model config values
// related to the models metadata. If no model is found for the provided uuid an
// error satisfying [modelerrors.NotFound] will be returned.
func (s *State) ModelMetadataDefaults(
	ctx context.Context,
	uuid coremodel.UUID,
) (map[string]string, error) {
	db, err := s.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	stmt := `
SELECT name, model_type_type
FROM v_model
WHERE uuid = ?
`

	var (
		modelName string
		modelType string
	)
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, stmt, uuid).Scan(&modelName, &modelType)
	})

	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w for uuid %q", modelerrors.NotFound, uuid)
	} else if err != nil {
		return nil, fmt.Errorf(
			"getting model metadata defaults for uuid %q: %w",
			uuid,
			domain.CoerceError(err),
		)
	}

	return map[string]string{
		config.NameKey: modelName,
		config.UUIDKey: uuid.String(),
		config.TypeKey: modelType,
	}, nil
}

// ModelProviderConfigSchema returns the providers config schema source based on
// the cloud set for the model. If no provider or schema source is found then
// an error satisfying errors.NotFound is returned. If the model is not found for
// the provided uuid then a error satisfying modelerrors.NotFound is returned.
func (s *State) ModelProviderConfigSchema(
	ctx context.Context,
	uuid coremodel.UUID,
) (config.ConfigSchemaSource, error) {
	db, err := s.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	cloudTypeStmt := `
SELECT cloud_type.type
FROM cloud_type
INNER JOIN cloud
ON cloud.cloud_type_id = cloud_type.id
INNER JOIN model m
ON m.cloud_uuid = cloud.uuid
WHERE m.uuid = ?
`

	var cloudType string
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, cloudTypeStmt, uuid).Scan(&cloudType)
	})

	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w %q", modelerrors.NotFound, uuid)
	} else if err != nil {
		return nil, fmt.Errorf("getting cloud type of model %q cloud: %w", uuid, err)
	}

	provider, err := environs.Provider(cloudType)
	if errors.Is(err, errors.NotFound) {
		return nil, fmt.Errorf(
			"model %q cloud type %q provider a schema source %w",
			uuid,
			cloudType,
			errors.NotFound,
		)
	} else if err != nil {
		return nil, fmt.Errorf("getting provider for model %q cloud type %q: %w", uuid, cloudType, err)
	}

	if cs, implements := provider.(config.ConfigSchemaSource); implements {
		return cs, nil
	}
	return nil, fmt.Errorf(
		"schema source for model %q with cloud type %q %w",
		uuid,
		cloudType,
		errors.NotFound,
	)
}

// NewState returns a new State for interacting with the underlying model
// defaults.
func NewState(factory database.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}
