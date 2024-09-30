// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/canonical/sqlair"
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

	mUUID := modelUUID{UUID: uuid.String()}

	stmt, err := s.Prepare(`
SELECT &cloudDefaults.*
FROM cloud_defaults
INNER JOIN cloud
ON cloud.uuid = cloud_defaults.cloud_uuid
INNER JOIN model m
ON m.cloud_uuid = cloud.uuid
WHERE m.uuid = $modelUUID.uuid
`, cloudDefaults{}, mUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var result []cloudDefaults
		if err := tx.Query(ctx, stmt, mUUID).GetAll(&result); errors.Is(err, sql.ErrNoRows) {
			return nil
		} else if err != nil {
			return fmt.Errorf("reading cloud defaults for model %q: %w", uuid, err)
		}

		for _, cd := range result {
			rval[cd.Key] = cd.Value
		}
		return nil
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

	mUUID := modelUUID{UUID: uuid.String()}

	stmt, err := s.Prepare(`
SELECT &cloudDefaults.*
FROM cloud_region_defaults
INNER JOIN cloud_region
ON cloud_region.uuid = cloud_region_defaults.region_uuid
INNER JOIN model m
ON m.cloud_region_uuid = cloud_region.uuid
WHERE m.uuid = $modelUUID.uuid
`, cloudDefaults{}, mUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var result []cloudDefaults
		if err := tx.Query(ctx, stmt, mUUID).GetAll(&result); errors.Is(err, sql.ErrNoRows) {
			return nil
		} else if err != nil {
			return fmt.Errorf("reading cloud region defaults for model %q: %w", uuid, err)
		}

		for _, cd := range result {
			rval[cd.Key] = cd.Value
		}
		return nil
	})

	if err != nil {
		return nil, err
	}
	return rval, nil
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

	mUUID := modelUUID{UUID: uuid.String()}
	var result modelMetadata
	stmt, err := s.Prepare(`
SELECT (m.name, ct.type) AS (&modelMetadata.*)
FROM model m
JOIN cloud c ON m.cloud_uuid = c.uuid
JOIN cloud_type ct ON c.cloud_type_id = ct.id
WHERE m.uuid = $modelUUID.uuid
`, result, mUUID)
	if err != nil {
		return nil, errors.Annotatef(err, "preparing select model metadata statement")
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, mUUID).Get(&result); errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w for uuid %q", modelerrors.NotFound, uuid)
		} else if err != nil {
			return fmt.Errorf(
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
		config.NameKey: result.Name,
		config.UUIDKey: uuid.String(),
		config.TypeKey: result.CloudType,
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

	mUUID := modelUUID{UUID: uuid.String()}

	cloudTypeStmt, err := s.Prepare(`
SELECT (cloud_type.type) AS (&cloudType.*)
FROM cloud_type
INNER JOIN cloud
ON cloud.cloud_type_id = cloud_type.id
INNER JOIN model m
ON m.cloud_uuid = cloud.uuid
WHERE m.uuid = $modelUUID.uuid
`, cloudType{}, mUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var cloudType cloudType
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, cloudTypeStmt, mUUID).Get(&cloudType); errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w %q", modelerrors.NotFound, uuid)
		} else if err != nil {
			return fmt.Errorf("getting cloud type of model %q cloud: %w", uuid, err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	provider, err := environs.Provider(cloudType.Type)
	if errors.Is(err, errors.NotFound) {
		return nil, fmt.Errorf(
			"model %q cloud type %q provider a schema source %w",
			uuid,
			cloudType,
			errors.NotFound,
		)
	} else if err != nil {
		return nil, fmt.Errorf("getting provider for model %q cloud type %q: %w", uuid, cloudType.Type, err)
	}

	if cs, implements := provider.(config.ConfigSchemaSource); implements {
		return cs, nil
	}
	return nil, fmt.Errorf(
		"schema source for model %q with cloud type %q %w",
		uuid,
		cloudType.Type,
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
