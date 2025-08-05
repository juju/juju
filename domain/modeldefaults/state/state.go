// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"

	"github.com/juju/juju/core/cloud"
	coredatabase "github.com/juju/juju/core/database"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain"
	clouderrors "github.com/juju/juju/domain/cloud/errors"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
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

// checkCloudExists checks if the cloud exists in the database as a helper func
// for a transaction. [clouderrors.NotFound] is returned if the cloud does not
// exist.
func (s *State) checkCloudExists(
	ctx context.Context,
	tx *sqlair.TX,
	cloudUUID cloud.UUID,
) error {
	cloudUUIDVal := cloudUUIDValue{UUID: cloudUUID.String()}
	cloudExistsStmt, err := s.Prepare(`
SELECT &cloudUUIDValue.*
FROM cloud
WHERE uuid = $cloudUUIDValue.uuid
`, cloudUUIDVal)
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, cloudExistsStmt, cloudUUIDVal).Get(&cloudUUIDVal)
	if errors.Is(err, sql.ErrNoRows) {
		return errors.Errorf(
			"cloud %q does not exist", cloudUUID,
		).Add(clouderrors.NotFound)
	} else if err != nil {
		return errors.Errorf("checking if cloud %q exists: %w", cloudUUID, err)
	}

	return nil
}

// CloudDefaults returns the defaults associated with the given cloud. If
// no defaults are found then an empty map will be returned with a nil error. If
// no cloud exists for the given id an error satisfying [clouderrors.NotFound]
// will be returned.
func (s *State) CloudDefaults(
	ctx context.Context,
	uuid cloud.UUID,
) (map[string]string, error) {
	rval := make(map[string]string)

	db, err := s.DB(ctx)
	if err != nil {
		return rval, errors.Capture(err)
	}

	cloudUUID := cloudUUIDValue{UUID: uuid.String()}
	cloudDefaultsStmt, err := s.Prepare(`
SELECT &keyValue.* 
FROM cloud_defaults
WHERE cloud_defaults.cloud_uuid = $cloudUUIDValue.uuid
`, keyValue{}, cloudUUID)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var kvs []keyValue
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := s.checkCloudExists(ctx, tx, uuid)
		if err != nil {
			return errors.Errorf("cannot get cloud %q defaults: %w", uuid, err)
		}

		err = tx.Query(ctx, cloudDefaultsStmt, cloudUUID).GetAll(&kvs)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("fetching cloud %q defaults: %w", uuid, err)
		}

		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	for _, kv := range kvs {
		rval[kv.Key] = kv.Value
	}

	return rval, nil
}

// ModelCloudRegionDefaults returns the defaults associated with the model's
// cloud region. It returns an error satisfying [modelerrors.NotFound] if the
// model doesn't exist.
func (s *State) ModelCloudRegionDefaults(
	ctx context.Context,
	uuid coremodel.UUID,
) (map[string]string, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	model := modelUUID{UUID: uuid.String()}

	modelExistsStmt, err := s.Prepare(`
SELECT &modelUUID.* FROM model WHERE uuid = $modelUUID.uuid
`, model)
	if err != nil {
		return nil, errors.Capture(err)
	}

	query, err := s.Prepare(`
SELECT (crd.key, crd.value) AS (&cloudRegionDefaultValue.*)
FROM cloud_region_defaults AS crd
JOIN cloud_region cr ON cr.uuid = crd.region_uuid
JOIN model m ON m.cloud_region_uuid = cr.uuid
WHERE m.uuid = $modelUUID.uuid
`, cloudRegionDefaultValue{}, model)
	if err != nil {
		return nil, errors.Capture(err)
	}

	result := map[string]string{}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, modelExistsStmt, model).Get(&model)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"getting model %q cloud region defaults, model does not exist", uuid,
			).Add(modelerrors.NotFound)
		} else if err != nil {
			return errors.Errorf(
				"checking if model %q exists: %w", uuid, err,
			)
		}

		var regionDefaultValues []cloudRegionDefaultValue

		err = tx.Query(ctx, query, model).GetAll(&regionDefaultValues)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"getting model %q cloud region defaults: %w", uuid, err,
			)
		}

		for _, regionDefaultValue := range regionDefaultValues {
			result[regionDefaultValue.Key] = regionDefaultValue.Value
		}
		return nil
	})

	if err != nil {
		return nil, err
	}
	return result, nil

}

// CloudAllRegionDefaults returns the defaults associated with all of the
// regions for the specified cloud. The result is a map of region name
// key values, keyed on the name of the region. If no defaults are found then an
// empty map will be returned with nil error. Note this will not include the
// defaults set on the cloud itself but just that of its regions.
//
// If no cloud exists for the given uuid an error satisfying
// [clouderrors.NotFound]
func (s *State) CloudAllRegionDefaults(
	ctx context.Context,
	cloudUUID cloud.UUID,
) (map[string]map[string]string, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	cloudUUIDVal := cloudUUIDValue{UUID: cloudUUID.String()}

	stmt, err := s.Prepare(`
SELECT (cloud_region.name,
        cloud_region_defaults.key,
		cloud_region_defaults.value) AS (&cloudRegionDefaultValue.*)
FROM cloud_region_defaults
JOIN cloud_region ON cloud_region.uuid = cloud_region_defaults.region_uuid
WHERE cloud_region.cloud_uuid = $cloudUUIDValue.uuid
`, cloudRegionDefaultValue{}, cloudUUIDVal)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var regionDefaultValues []cloudRegionDefaultValue
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := s.checkCloudExists(ctx, tx, cloudUUID)
		if err != nil {
			return errors.Errorf(
				"getting cloud %q all region defaults: %w", cloudUUID, err,
			)
		}

		err = tx.Query(ctx, stmt, cloudUUIDVal).GetAll(&regionDefaultValues)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"getting cloud %q all region defaults: %w", cloudUUID, err,
			)
		}
		return nil
	})

	if err != nil {
		return nil, errors.Capture(err)
	}

	defaults := map[string]map[string]string{}
	for _, regionDefaultValue := range regionDefaultValues {
		store, has := defaults[regionDefaultValue.RegionName]
		if !has {
			store = map[string]string{}
			defaults[regionDefaultValue.RegionName] = store
		}
		store[regionDefaultValue.Key] = regionDefaultValue.Value
	}

	return defaults, nil
}

// CloudType returns the cloud type of the cloud.
// If no cloud exists for the given uuid then an error
// satisfying [clouderrors.NotFound] is returned.
func (s *State) CloudType(
	ctx context.Context,
	uuid cloud.UUID,
) (string, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return "'", errors.Capture(err)
	}

	cldUUIDVal := cloudUUIDValue{UUID: uuid.String()}
	result := modelCloudType{}

	stmt, err := s.Prepare(`
SELECT ct.type AS &modelCloudType.cloud_type
FROM cloud AS c
JOIN cloud_type AS ct ON ct.id = c.cloud_type_id
WHERE c.uuid = $cloudUUIDValue.uuid
`, cldUUIDVal, result)

	if err != nil {
		return "", errors.Errorf("preparing model cloud type select statement: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, cldUUIDVal).Get(&result)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"cannot get cloud type for cloud %q because cloud does not exist",
				uuid,
			).Add(clouderrors.NotFound)
		} else if err != nil {
			return errors.Errorf(
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
	db, err := s.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
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
		return nil, errors.Errorf("preparing select model metadata statement: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, modelUUID).Get(&result)
		if errors.Is(err, sql.ErrNoRows) {
			return errors.Errorf("%w for uuid %q", modelerrors.NotFound, uuid)
		} else if err != nil {
			return errors.Errorf(
				"getting model metadata defaults for uuid %q: %w",
				uuid,
				err,
			)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	return map[string]string{
		config.NameKey: result.ModelName,
		config.UUIDKey: uuid.String(),
		config.TypeKey: result.CloudType,
	}, nil
}

// GetModelCloudUUID returns the cloud UUID for the given model.
// If the model is not found, an error specifying
// [modelerrors.NotFound] is returned.
func (s *State) GetModelCloudUUID(ctx context.Context, uuid coremodel.UUID) (cloud.UUID, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	model := modelUUID{UUID: uuid.String()}

	query, err := s.Prepare(`
SELECT m.cloud_uuid AS &cloudUUIDValue.uuid
FROM model m
WHERE m.uuid = $modelUUID.uuid
`, model, cloudUUIDValue{})
	if err != nil {
		return "", errors.Capture(err)
	}

	var cld cloudUUIDValue
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, query, model).Get(&cld)
		if errors.Is(err, sqlair.ErrNoRows) {
			return modelerrors.NotFound
		}
		return err
	})
	if err != nil {
		return "", errors.Errorf("getting cloud uuid for model %q: %w", uuid, err)
	}
	return cloud.UUID(cld.UUID), nil
}

// GetCloudUUID returns the cloud UUID and region for the given cloud name.
// If the cloud is not found, an error specifying [clouderrors.NotFound] is
// returned.
func (s *State) GetCloudUUID(ctx context.Context, cloudName string) (cloud.UUID, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}
	cloudNameVal := cloudNameValue{Name: cloudName}
	query, err := s.Prepare(`
SELECT &cloudUUIDValue.*
FROM cloud c
WHERE c.name = $cloudNameValue.name
`, cloudNameVal, cloudUUIDValue{})
	if err != nil {
		return "", errors.Capture(err)
	}

	cloudUUIDVal := cloudUUIDValue{}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, query, cloudNameVal).Get(&cloudUUIDVal)
		if errors.Is(err, sqlair.ErrNoRows) {
			return clouderrors.NotFound
		}
		return err
	})

	if err != nil {
		return cloud.UUID(""), errors.Errorf(
			"getting cloud UUID for cloud with name %q: %w", cloudName, err,
		)
	}

	return cloud.UUID(cloudUUIDVal.UUID), nil
}

// SetCloudDefaults is responsible for removing any previously set cloud
// default values and setting the new cloud defaults to use. If no defaults are
// supplied to this function then the currently set cloud default values will be
// removed and no further operations will be performed. If no cloud exists for
// the cloud name then an error satisfying [clouderrors.NotFound] is returned.
func SetCloudDefaults(
	ctx context.Context,
	tx *sqlair.TX,
	cloudName string,
	defaults map[string]string,
) error {
	cloudNameVal := cloudNameValue{Name: cloudName}
	var cloudUUIDVal cloudUUIDValue
	cloudUUIDStmt, err := sqlair.Prepare(`
SELECT &cloudUUIDValue.*
FROM cloud 
WHERE name = $cloudNameValue.name
`, cloudNameVal, cloudUUIDVal)
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, cloudUUIDStmt, cloudNameVal).Get(&cloudUUIDVal)
	if errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf(
			"setting cloud %q defaults, cloud does not exist", cloudName,
		).Add(clouderrors.NotFound)
	} else if err != nil {
		return errors.Errorf(
			"getting cloud %q uuid to set cloud defaults: %w", cloudName, err,
		)
	}

	deleteStmt, err := sqlair.Prepare(`
DELETE FROM cloud_defaults 
WHERE       cloud_defaults.cloud_uuid = $cloudUUIDValue.uuid
`, cloudUUIDVal)
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, deleteStmt, cloudUUIDVal).Run()
	if err != nil {
		return errors.Errorf("removing previously set cloud %q defaults: %w", cloudName, err)
	}

	if len(defaults) == 0 {
		return nil
	}

	dbDefaults := transform.MapToSlice(defaults, func(k, v string) []cloudDefaultValue {
		return []cloudDefaultValue{{UUID: cloudUUIDVal.UUID, Key: k, Value: v}}
	})

	insertStmt, err := sqlair.Prepare(`
INSERT INTO cloud_defaults (cloud_uuid, key, value) 
VALUES ($cloudDefaultValue.*)`, cloudDefaultValue{})
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, insertStmt, dbDefaults).Run()
	if err != nil {
		return errors.Errorf("setting cloud %q defaults: %w", cloudName, err)
	}

	return nil
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
	db, err := s.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	upsertStmt, err := sqlair.Prepare(`
INSERT INTO cloud_defaults (cloud_uuid, key, value)
VALUES ($cloudDefaultValue.*)
ON CONFLICT(cloud_uuid, key) DO UPDATE
    SET value = excluded.value
    WHERE cloud_uuid = excluded.cloud_uuid
    AND key = excluded.key;
`, cloudDefaultValue{})
	if err != nil {
		return errors.Capture(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		for k, v := range updateAttrs {
			err := tx.Query(ctx, upsertStmt, cloudDefaultValue{UUID: cloudUUID.String(), Key: k, Value: v}).Run()
			// The cloud UUID has previously been checked. This allows us to avoid having to use RunAtomic.
			if database.IsErrConstraintForeignKey(err) {
				return errors.Errorf("cloud %q not found", cloudUUID).Add(clouderrors.NotFound)
			} else if err != nil {
				return errors.Errorf("updating cloud %q default keys: %w", cloudUUID, err)
			}
		}
		return nil
	})
}

// DeleteCloudDefaults will delete the specified default keys from the cloud if
// they exist. If the cloud does not exist an error satisfying
// [clouderrors.NotFound] will be returned.
func (s *State) DeleteCloudDefaults(
	ctx context.Context,
	cloudUUID cloud.UUID,
	removeAttrs []string,
) error {
	db, err := s.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	cld := cloudUUIDValue{UUID: cloudUUID.String()}
	toRemove := attrs(removeAttrs)

	deleteStmt, err := s.Prepare(`
DELETE FROM cloud_defaults
WHERE key IN ($attrs[:])
AND cloud_uuid = $cloudUUIDValue.uuid;
`, toRemove, cld)
	if err != nil {
		return errors.Capture(err)
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := s.checkCloudExists(ctx, tx, cloudUUID); err != nil {
			return err
		}
		return tx.Query(ctx, deleteStmt, toRemove, cld).Run()
	})
	if err != nil {
		return errors.Errorf("removing cloud %q default keys: %w", cloudUUID, err)
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
	db, err := s.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	cld := cloudUUIDValue{UUID: cloudUUID.String()}
	region := cloudRegion{Name: regionName}

	selectStmt, err := s.Prepare(`
SELECT &cloudRegion.uuid
FROM cloud_region
WHERE cloud_region.cloud_uuid = $cloudUUIDValue.uuid
AND cloud_region.name = $cloudRegion.name;
`, region, cld)
	if err != nil {
		return errors.Capture(err)
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
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, selectStmt, cld, region).Get(&region)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"cloud %q region %q does not exist",
				cloudUUID, regionName,
			).Add(clouderrors.NotFound)
		} else if err != nil {
			return errors.Errorf("fetching cloud %q region %q: %w", cloudUUID, regionName, err)
		}

		for k, v := range updateAttrs {
			err := tx.Query(ctx, upsertStmt, cloudRegionDefaultValue{UUID: region.UUID, Key: k, Value: v}).Run()
			// The cloud UUID has previously been checked. This allows us to avoid having to use RunAtomic.
			if database.IsErrConstraintForeignKey(err) {
				return errors.Errorf("cloud %q not found", cloudUUID).Add(clouderrors.NotFound)
			} else if err != nil {
				return errors.Capture(err)
			}
		}
		return nil
	})
	if err != nil {
		return errors.Errorf(
			"updating cloud %q region %q default keys: %w",
			cloudUUID,
			regionName,
			err,
		)
	}
	return nil
}

// DeleteCloudRegionDefaults deletes the specified default config keys for the
// given cloud region. It returns an error satisfying [clouderrors.NotFound] if
// the region doesn't exist.
func (s *State) DeleteCloudRegionDefaults(
	ctx context.Context,
	cloudUUID cloud.UUID,
	regionName string,
	removeAttrs []string,
) error {
	db, err := s.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	cld := cloudUUIDValue{UUID: cloudUUID.String()}
	region := cloudRegion{Name: regionName}

	selectStmt, err := s.Prepare(`
SELECT &cloudRegion.uuid
FROM cloud_region
WHERE cloud_region.cloud_uuid = $cloudUUIDValue.uuid
AND cloud_region.name = $cloudRegion.name;
`, region, cld)
	if err != nil {
		return errors.Capture(err)
	}

	toRemove := attrs(removeAttrs)

	deleteStmt, err := s.Prepare(`
DELETE FROM  cloud_region_defaults
WHERE key IN ($attrs[:])
AND region_uuid = $cloudRegion.uuid;
`, toRemove, region)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, selectStmt, cld, region).Get(&region)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.New("cloud region does not exist").Add(clouderrors.NotFound)
		} else if err != nil {
			return err
		}

		err = tx.Query(ctx, deleteStmt, region, toRemove).Run()
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return errors.Errorf("removing cloud %q region %q defaults: %w", cloudUUID, regionName, err)
	}

	return nil
}
