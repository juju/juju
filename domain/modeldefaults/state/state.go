// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	"github.com/juju/juju/core/database"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/environs/config"
	interrors "github.com/juju/juju/internal/errors"
)

// State represents a type for interacting with the underlying model defaults
// state.
type State struct {
	*domain.StateBase
}

// ConfigDefaults returns the default configuration values set in Juju.
func ConfigDefaults(_ context.Context) map[string]any {
	return config.ConfigDefaults()
}

// ConfigDefaults returns the default configuration values set in Juju.
func (s *State) ConfigDefaults(ctx context.Context) map[string]any {
	return ConfigDefaults(ctx)
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
			return interrors.Errorf("fetching cloud defaults for model %q: %w", uuid, err)
		}
		defer rows.Close()

		for rows.Next() {
			var key, val string
			if err := rows.Scan(&key, &val); err != nil {
				return interrors.Errorf("reading cloud defaults for model %q: %w", uuid, err)
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
			return interrors.Errorf("fetching cloud region defaults for model %q: %w", uuid, err)
		}
		defer rows.Close()

		var (
			key, val string
		)
		for rows.Next() {
			if err := rows.Scan(&key, &val); err != nil {
				return interrors.Errorf("reading cloud region defaults for model %q: %w", uuid, err)
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

// ModelCloudType returns the cloud type for model identified by the given model
// uuid. If no model exists for the provided model uuid then an error satisfying
// [modelerrors.NotFound] is returned.
func (s *State) ModelCloudType(
	ctx context.Context,
	uuid coremodel.UUID,
) (string, error) {
	db, err := s.DB()
	if err != nil {
		return "'", errors.Trace(err)
	}

	modelUUIDVal := modelUUIDValue{UUID: uuid.String()}
	result := modelCloudType{}

	stmt, err := s.Prepare(`
SELECT (ct.type) AS (&modelCloudType.cloud_type)
FROM model AS m
JOIN cloud AS c ON c.uuid = m.cloud_uuid
JOIN cloud_type AS ct ON ct.id = c.cloud_type_id 
WHERE m.uuid = $modelUUIDValue.model_uuid
`, modelUUIDVal, result)

	if err != nil {
		return "", interrors.Errorf("preparing model cloud type select statement: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, modelUUIDVal).Get(&result)
		if errors.Is(err, sqlair.ErrNoRows) {
			return interrors.Errorf(
				"cannot get cloud type for model %q because model does not exist",
				uuid,
			).Add(modelerrors.NotFound)
		} else if err != nil {
			return interrors.Errorf(
				"cannot get cloud type for model %q: %w", uuid, err,
			)
		}

		return nil
	})

	if err != nil {
		return "", err
	}

	return result.CloudType, nil
}

// ModelMetadataDefaults is responsible for providing metadata defaults for a
// model's config. These include things like the model's name and uuid.
// If no model exists for the provided uuid then a [modelerrors.NotFound] error
// is returned.
func (s *State) ModelMetadataDefaults(
	ctx context.Context,
	uuid coremodel.UUID,
) (map[string]string, error) {
	db, err := s.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	stmt := `
SELECT m.name, ct.type
FROM model m
JOIN cloud c ON m.cloud_uuid = c.uuid
JOIN cloud_type ct ON c.cloud_type_id = ct.id
WHERE m.uuid = ?
`

	var (
		modelName string
		cloudType string
	)
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, stmt, uuid).Scan(&modelName, &cloudType)
		if errors.Is(err, sql.ErrNoRows) {
			return interrors.Errorf("%w for uuid %q", modelerrors.NotFound, uuid)
		} else if err != nil {
			return interrors.Errorf(
				"getting model metadata defaults for uuid %q: %w",
				uuid,
				err,
			)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	return map[string]string{
		config.NameKey: modelName,
		config.UUIDKey: uuid.String(),
		config.TypeKey: cloudType,
	}, nil
}

// NewState returns a new State for interacting with the underlying model
// defaults.
func NewState(factory database.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}
