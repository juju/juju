// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	"github.com/juju/juju/core/cloud"
	coredatabase "github.com/juju/juju/core/database"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain"
	clouderrors "github.com/juju/juju/domain/cloud/errors"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/database"
	interrors "github.com/juju/juju/internal/errors"
)

// State represents a type for interacting with the underlying model defaults
// state.
type State struct {
	*domain.StateBase
}

// NewState returns a new State for interacting with the underlying model
// defaults.
func NewState(factory coredatabase.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}

// ConfigDefaults returns the default configuration values set in Juju.
func ConfigDefaults(_ context.Context) map[string]any {
	return config.ConfigDefaults()
}

// ConfigDefaults returns the default configuration values set in Juju.
func (s *State) ConfigDefaults(ctx context.Context) map[string]any {
	return ConfigDefaults(ctx)
}

// CloudDefaults returns the defaults associated with the given cloud. If
// no defaults are found then an empty map will be returned with a nil error.
func (s *State) CloudDefaults(
	ctx context.Context,
	uuid cloud.UUID,
) (map[string]string, error) {
	rval := make(map[string]string)

	db, err := s.DB()
	if err != nil {
		return rval, errors.Trace(err)
	}

	cloudUUID := cloudUUID{UUID: uuid.String()}
	cloudDefaultsStmt, err := s.Prepare(`
SELECT &keyValue.* 
FROM cloud_defaults
WHERE cloud_defaults.cloud_uuid = $cloudUUID.uuid
`, keyValue{}, cloudUUID)
	if err != nil {
		return nil, interrors.Errorf("preparing select cloud defaults statement: %w", err)
	}

	var kvs []keyValue
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, cloudDefaultsStmt, cloudUUID).GetAll(&kvs)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return interrors.Errorf("fetching cloud defaults for model %q: %w", uuid, err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	for _, kv := range kvs {
		rval[kv.Key] = kv.Value
	}

	return rval, nil
}

// ModelCloudRegionDefaults returns the defaults associated with the model's cloud region.
// It returns an error satisfying [modelerrors.NotFound] if the model doesn't exist.
func (s *State) ModelCloudRegionDefaults(ctx context.Context, uuid coremodel.UUID) (map[string]string, error) {
	db, err := s.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	model := modelUUID{UUID: uuid.String()}

	query, err := s.Prepare(`
SELECT (crd.key, crd.value) AS (&cloudRegionDefaultValue.*)
FROM model m
JOIN cloud_region cr ON cr.uuid = m.cloud_region_uuid
LEFT JOIN cloud_region_defaults crd ON crd.region_uuid = cr.uuid
WHERE m.uuid = $modelUUID.uuid
`, cloudRegionDefaultValue{}, model)
	if err != nil {
		return nil, errors.Trace(err)
	}

	result := make(map[string]string)

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {

		var regionDefaultValues []cloudRegionDefaultValue

		err := tx.Query(ctx, query, model).GetAll(&regionDefaultValues)
		if errors.Is(err, sqlair.ErrNoRows) {
			return interrors.Errorf("model %q not found", uuid).Add(modelerrors.NotFound)
		}
		if err != nil {
			return errors.Trace(err)
		}

		for _, regionDefaultValue := range regionDefaultValues {
			if regionDefaultValue.Key == "" {
				continue
			}
			result[regionDefaultValue.Key] = regionDefaultValue.Value
		}
		return nil
	})
	if err != nil {
		return nil, errors.Annotatef(err, "getting cloud region defaults details for model %q", uuid)
	}
	return result, nil

}

// CloudAllRegionDefaults returns the defaults associated with all of the
// regions for the specified cloud. The result is a map of region name
// key values, keyed on the name of the region.
// If no defaults are found then an empty map will be returned with nil error.
// Note this will not include the defaults set on the cloud itself but
// just that of its regions.
func (s *State) CloudAllRegionDefaults(
	ctx context.Context,
	cloudUUID cloud.UUID,
) (map[string]map[string]string, error) {
	defaults := map[string]map[string]string{}

	db, err := s.DB()
	if err != nil {
		return defaults, interrors.Errorf("getting database instance for cloud region defaults: %w", err)
	}

	stmt, err := s.Prepare(`
SELECT (cloud_region.name, cloud_region_defaults.key, cloud_region_defaults.value) AS (&cloudRegionDefaultValue.*)
FROM cloud_region_defaults
JOIN cloud_region ON cloud_region.uuid = cloud_region_defaults.region_uuid
WHERE cloud_region.cloud_uuid = $dbCloud.uuid
`, cloudRegionDefaultValue{}, dbCloud{})
	if err != nil {
		return defaults, errors.Trace(err)
	}

	return defaults, db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {

		var regionDefaultValues []cloudRegionDefaultValue

		if err := tx.Query(ctx, stmt, dbCloud{UUID: cloudUUID.String()}).GetAll(&regionDefaultValues); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return nil
			}
			return interrors.Errorf("fetching cloud %q region defaults: %w", cloudUUID, err)
		}

		for _, regionDefaultValue := range regionDefaultValues {
			store, has := defaults[regionDefaultValue.RegionName]
			if !has {
				store = map[string]string{}
				defaults[regionDefaultValue.RegionName] = store
			}
			store[regionDefaultValue.Key] = regionDefaultValue.Value
		}
		return nil
	})
}

// CloudType returns the cloud type of the cloud.
// If no cloud exists for the given uuid then an error
// satisfying [clouderrors.NotFound] is returned.
func (s *State) CloudType(
	ctx context.Context,
	uuid cloud.UUID,
) (string, error) {
	db, err := s.DB()
	if err != nil {
		return "'", errors.Trace(err)
	}

	cld := dbCloud{UUID: uuid.String()}
	result := modelCloudType{}

	stmt, err := s.Prepare(`
SELECT ct.type AS &modelCloudType.cloud_type
FROM cloud AS c
JOIN cloud_type AS ct ON ct.id = c.cloud_type_id
WHERE c.uuid = $dbCloud.uuid
`, cld, result)

	if err != nil {
		return "", interrors.Errorf("preparing model cloud type select statement: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, cld).Get(&result)
		if errors.Is(err, sqlair.ErrNoRows) {
			return interrors.Errorf(
				"cannot get cloud type for cloud %q because cloud does not exist",
				uuid,
			).Add(clouderrors.NotFound)
		} else if err != nil {
			return interrors.Errorf(
				"cannot get cloud type for cloud %q: %w", uuid, err,
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

	modelUUID := modelUUID{UUID: uuid.String()}
	var result modelMetadata
	stmt, err := s.Prepare(`
SELECT (m.name, ct.type) AS (&modelMetadata.*)
FROM model m
JOIN cloud c ON m.cloud_uuid = c.uuid
JOIN cloud_type ct ON c.cloud_type_id = ct.id
WHERE m.uuid = $modelUUID.uuid
`, result, modelUUID)
	if err != nil {
		return nil, interrors.Errorf("preparing select model metadata statement: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, modelUUID).Get(&result)
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
		config.NameKey: result.ModelName,
		config.UUIDKey: uuid.String(),
		config.TypeKey: result.CloudType,
	}, nil
}

// GetModelCloudUUID returns the cloud UUID for the given model.
// If the model is not found, an error specifying [modelerrors.NotFound] is returned.
func (s *State) GetModelCloudUUID(ctx context.Context, uuid coremodel.UUID) (cloud.UUID, error) {
	db, err := s.DB()
	if err != nil {
		return "", errors.Trace(err)
	}

	model := modelUUID{UUID: uuid.String()}

	query, err := s.Prepare(`
SELECT m.cloud_uuid AS &dbCloud.uuid
FROM model m
WHERE m.uuid = $modelUUID.uuid
`, model, dbCloud{})
	if err != nil {
		return "", errors.Trace(err)
	}

	var cld dbCloud
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, query, model).Get(&cld)
		if errors.Is(err, sqlair.ErrNoRows) {
			return interrors.Errorf("model %q not found", uuid).Add(modelerrors.NotFound)
		}
		return errors.Trace(err)
	})
	if err != nil {
		return "", errors.Annotatef(err, "getting cloud UUID for model %q", uuid)
	}
	return cloud.UUID(cld.UUID), nil
}

// GetCloudUUID returns the cloud UUID and region for the given cloud name.
// If the cloud is not found, an error specifying [clouderrors.NotFound] is returned.
func (s *State) GetCloudUUID(ctx context.Context, cloudName string) (cloud.UUID, error) {
	db, err := s.DB()
	if err != nil {
		return "", errors.Trace(err)
	}
	cld := dbCloud{Name: cloudName}
	query, err := s.Prepare(`
SELECT &dbCloud.uuid
FROM cloud c
WHERE c.name = $dbCloud.name
`, cld)
	if err != nil {
		return "", errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, query, cld).Get(&cld)
		if errors.Is(err, sqlair.ErrNoRows) {
			return interrors.Errorf("cloud %q not found", cloudName).Add(clouderrors.NotFound)
		}
		return errors.Trace(err)
	})
	return cloud.UUID(cld.UUID), errors.Annotatef(err, "getting cloud UUID for %q", cloudName)
}

// UpdateCloudDefaults is responsible for updating default config values for a
// cloud. This function will allow the addition and updating of attributes.
// If the cloud doesn't exist, an error satisfying [clouderrors.NotFound]
// is returned.
func (s *State) UpdateCloudDefaults(
	ctx context.Context,
	cloudUUID cloud.UUID,
	updateAttrs map[string]string,
) error {
	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
	}

	cld := dbCloud{UUID: cloudUUID.String()}

	upsertStmt, err := sqlair.Prepare(`
INSERT INTO cloud_defaults (cloud_uuid, key, value)
VALUES ($cloudDefaultValue.*)
ON CONFLICT(cloud_uuid, key) DO UPDATE
    SET value = excluded.value
    WHERE cloud_uuid = excluded.cloud_uuid
    AND key = excluded.key;
`, cloudDefaultValue{})
	if err != nil {
		return errors.Trace(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		for k, v := range updateAttrs {
			err := tx.Query(ctx, upsertStmt, cloudDefaultValue{UUID: cld.UUID, Key: k, Value: v}).Run()
			// The cloud UUID has previously been checked. This allows us to avoid having to use RunAtomic.
			if database.IsErrConstraintForeignKey(err) {
				return interrors.Errorf("cloud %q not found", cloudUUID).Add(clouderrors.NotFound)
			} else if err != nil {
				return interrors.Errorf("updating %q cloud default keys: %w", cloudUUID, err)
			}
		}
		return nil
	})
}

// DeleteCloudDefaults deletes the specified cloud default
// config values for the provided keys if they exist.
func (s *State) DeleteCloudDefaults(
	ctx context.Context,
	cloudUUID cloud.UUID,
	removeAttrs []string,
) error {
	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
	}

	cld := dbCloud{UUID: cloudUUID.String()}
	toRemove := attrs(removeAttrs)

	deleteStmt, err := s.Prepare(`
DELETE FROM cloud_defaults
WHERE key IN ($attrs[:])
AND cloud_uuid = $dbCloud.uuid;
`, toRemove, cld)
	if err != nil {
		return errors.Trace(err)
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, deleteStmt, toRemove, cld).Run()
	})
	if err != nil {
		return interrors.Errorf("removing cloud %q default keys: %w", cloudUUID, err)
	}
	return nil
}

// UpdateCloudRegionDefaults is responsible for updating default config values
// for a cloud region. This function will allow the addition and updating of
// attributes. If the cloud region is not found an error satisfying
// [clouderrors.NotFound] is returned.
func (s *State) UpdateCloudRegionDefaults(
	ctx context.Context,
	cloudUUID cloud.UUID,
	regionName string,
	updateAttrs map[string]string,
) error {
	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
	}

	cld := dbCloud{UUID: cloudUUID.String()}
	region := cloudRegion{Name: regionName}

	selectStmt, err := s.Prepare(`
SELECT &cloudRegion.uuid
FROM cloud_region
WHERE cloud_region.cloud_uuid = $dbCloud.uuid
AND cloud_region.name = $cloudRegion.name;
`, region, cld)
	if err != nil {
		return errors.Trace(err)
	}

	upsertStmt, err := s.Prepare(`
INSERT INTO cloud_region_defaults (region_uuid, key, value)
VALUES ($cloudRegionDefaultValue.*)
ON CONFLICT(region_uuid, key) DO UPDATE
    SET value = excluded.value
    WHERE region_uuid = excluded.region_uuid
    AND key = excluded.key;
`, cloudRegionDefaultValue{})
	if err != nil {
		return errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, selectStmt, cld, region).Get(&region)
		if errors.Is(err, sqlair.ErrNoRows) {
			return interrors.Errorf(
				"cloud %q region %q does not exist",
				cloudUUID, regionName,
			).Add(clouderrors.NotFound)
		} else if err != nil {
			return interrors.Errorf("fetching cloud %q region %q: %w", cloudUUID, regionName, err)
		}

		for k, v := range updateAttrs {
			err := tx.Query(ctx, upsertStmt, cloudRegionDefaultValue{UUID: region.UUID, Key: k, Value: v}).Run()
			// The cloud UUID has previously been checked. This allows us to avoid having to use RunAtomic.
			if database.IsErrConstraintForeignKey(err) {
				return interrors.Errorf("cloud %q not found", cloudUUID).Add(clouderrors.NotFound)
			} else if err != nil {
				return errors.Trace(err)
			}
		}
		return nil
	})
	if err != nil {
		return interrors.Errorf(
			"updating cloud %q region %q default keys: %w",
			cloudUUID,
			regionName,
			err,
		)
	}
	return nil
}

// DeleteCloudRegionDefaults deletes the specified default config
// keys for the given cloud region.
// It returns an error satisfying [errors.NotFound] if the
// region doesn't exist.
func (s *State) DeleteCloudRegionDefaults(
	ctx context.Context,
	cloudUUID cloud.UUID,
	regionName string,
	removeAttrs []string,
) error {
	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
	}

	cld := dbCloud{UUID: cloudUUID.String()}
	region := cloudRegion{Name: regionName}

	selectStmt, err := s.Prepare(`
SELECT &cloudRegion.uuid
FROM cloud_region
WHERE cloud_region.cloud_uuid = $dbCloud.uuid
AND cloud_region.name = $cloudRegion.name;
`, region, cld)
	if err != nil {
		return errors.Trace(err)
	}

	toRemove := attrs(removeAttrs)

	deleteStmt, err := s.Prepare(`
DELETE FROM  cloud_region_defaults
WHERE key IN ($attrs[:])
AND region_uuid = $cloudRegion.uuid;
`, toRemove, region)
	if err != nil {
		return errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, selectStmt, cld, region).Get(&region)
		if errors.Is(err, sqlair.ErrNoRows) {
			return interrors.Errorf("cloud %q region %q %w", cloudUUID, regionName, errors.NotFound)
		} else if err != nil {
			return interrors.Errorf("fetching cloud %q region %q: %w", cloudUUID, regionName, err)
		}
		return tx.Query(ctx, deleteStmt, region, toRemove).Run()
	})
	if err != nil {
		return interrors.Errorf(
			"removing cloud %q region %q default keys: %w",
			cloudUUID,
			regionName,
			err,
		)
	}
	return nil
}
