// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/changestream"
	corecloud "github.com/juju/juju/core/cloud"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain"
	accesserrors "github.com/juju/juju/domain/access/errors"
	clouderrors "github.com/juju/juju/domain/cloud/errors"
	"github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/uuid"
)

// State is used to access the database.
type State struct {
	*domain.StateBase
}

// NewState creates a state to access the database.
func NewState(factory coredatabase.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}

// CloudSupportsAuthType allows the caller to ask if a given auth type is
// currently supported by the cloud named by cloudName. If no cloud is found for
// the provided name an error matching [clouderrors.NotFound] is returned.
func CloudSupportsAuthType(
	ctx context.Context,
	tx *sqlair.TX,
	cloudName string,
	authType cloud.AuthType,
) (bool, error) {

	cloudStmt := `
SELECT cloud.uuid AS &M.cloudUUID
FROM cloud
WHERE cloud.name = $M.cloudName
`

	selectCloudStmt, err := sqlair.Prepare(cloudStmt, sqlair.M{})
	if err != nil {
		return false, errors.Trace(err)
	}

	var result = sqlair.M{}
	err = tx.Query(ctx, selectCloudStmt, sqlair.M{"cloudName": cloudName}).Get(&result)
	if errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("%w %q", clouderrors.NotFound, cloudName)
	} else if err != nil {
		return false, fmt.Errorf(
			"determining if cloud %q supports auth type %q: %w",
			cloudName, authType.String(), domain.CoerceError(err),
		)
	}

	authTypeStmt := `
SELECT auth_type.type AS &M.supports
FROM cloud
INNER JOIN cloud_auth_type
ON cloud.uuid = cloud_auth_type.cloud_uuid
INNER JOIN auth_type
ON cloud_auth_type.auth_type_id = auth_type.id
WHERE cloud.uuid = $M.cloudUUID
AND auth_type.type = $M.authType
`
	selectCloudAuthTypeStmt, err := sqlair.Prepare(authTypeStmt, sqlair.M{})
	if err != nil {
		return false, errors.Trace(err)
	}

	err = tx.Query(ctx, selectCloudAuthTypeStmt, sqlair.M{
		"cloudUUID": result["cloudUUID"],
		"authType":  authType.String(),
	}).Get(&result)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf(
			"determining if cloud %q supports auth type %q: %w",
			cloudName, authType.String(), domain.CoerceError(err),
		)
	}

	return true, nil
}

// ListClouds lists the clouds with the specified filter, if any.
func (st *State) ListClouds(ctx context.Context) ([]cloud.Cloud, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var result []cloud.Cloud
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		result, err = LoadClouds(ctx, st, tx, "")
		return errors.Trace(err)
	})
	return result, errors.Trace(err)
}

// Cloud returns the cloud with the specified name.
func (st *State) Cloud(ctx context.Context, name string) (*cloud.Cloud, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var result *cloud.Cloud
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		clouds, err := LoadClouds(ctx, st, tx, name)
		if err != nil {
			return errors.Trace(err)
		}
		if len(clouds) == 0 {
			return fmt.Errorf("%w %q", clouderrors.NotFound, name)
		}
		result = &clouds[0]
		return nil
	})
	return result, errors.Trace(err)
}

// GetCloudForID returns the cloud associated with the provided id. If no cloud is
// found for the given id then a [clouderrors.NotFound] error is returned.
func (st *State) GetCloudForID(ctx context.Context, id corecloud.ID) (cloud.Cloud, error) {
	db, err := st.DB()
	if err != nil {
		return cloud.Cloud{}, errors.Trace(err)
	}

	var rval cloud.Cloud
	return rval, db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		rval, err = GetCloudForID(ctx, st, tx, id)
		return err
	})
}

// GetCloudForID returns the cloud associated with the provided id. If no cloud is
// found for the given id then a [clouderrors.NotFound] error is returned.
func GetCloudForID(
	ctx context.Context,
	st domain.Preparer,
	tx *sqlair.TX,
	id corecloud.ID,
) (cloud.Cloud, error) {
	q := `
	SELECT (uuid, name, cloud_type, cloud_type_id, endpoint,
            identity_endpoint, storage_endpoint, skip_tls_verify,
            is_controller_cloud, auth_type) AS (&CloudWithAuthType.*)
    FROM v_cloud_auth
	WHERE uuid = $M.cloud_uuid
`

	stmt, err := st.Prepare(q, sqlair.M{}, CloudWithAuthType{})
	if err != nil {
		return cloud.Cloud{}, errors.Trace(err)
	}

	args := sqlair.M{
		"cloud_uuid": id.String(),
	}

	records := []CloudWithAuthType{}
	err = tx.Query(ctx, stmt, args).GetAll(&records)
	if errors.Is(err, sql.ErrNoRows) {
		return cloud.Cloud{}, fmt.Errorf("%w for uuid %q", clouderrors.NotFound, id)
	} else if err != nil {
		return cloud.Cloud{}, fmt.Errorf("getting cloud %q: %w", id, domain.CoerceError(err))
	}

	cld := cloud.Cloud{
		Name:              records[0].Name,
		Type:              records[0].Type,
		Endpoint:          records[0].Endpoint,
		IdentityEndpoint:  records[0].IdentityEndpoint,
		StorageEndpoint:   records[0].StorageEndpoint,
		SkipTLSVerify:     records[0].SkipTLSVerify,
		IsControllerCloud: records[0].IsControllerCloud,
		AuthTypes:         make(cloud.AuthTypes, 0, len(records)),
		Regions:           []cloud.Region{},
		CACertificates:    []string{},
	}
	for _, record := range records {
		cld.AuthTypes = append(cld.AuthTypes, cloud.AuthType(record.AuthType))
	}

	caCerts, err := loadCACerts(ctx, tx, []string{id.String()})
	if err != nil {
		return cloud.Cloud{}, fmt.Errorf("loading cloud %q ca certificates: %w", id, err)
	}
	cld.CACertificates = caCerts[id.String()]

	regions, err := loadRegions(ctx, tx, []string{id.String()})
	if err != nil {
		return cloud.Cloud{}, fmt.Errorf("loading cloud %q regions: %w", id, err)
	}
	cld.Regions = regions[id.String()]

	return cld, nil
}

// CloudDefaults provides the currently set cloud defaults for a cloud. If the
// cloud has no defaults or the cloud does not exist a nil error is returned
// with an empty defaults map.
func (st *State) CloudDefaults(ctx context.Context, cloudName string) (map[string]string, error) {
	db, err := st.DB()
	if err != nil {
		return nil, fmt.Errorf("getting database for setting cloud %q defaults: %w", cloudName, err)
	}

	// This might look like an odd way to query for cloud defaults but by doing
	// a left join onto the cloud table we are always guaranteed at least one
	// row to be returned. This lets us confirm that a cloud actually exists
	// for the name.
	// The reason for going to so much effort for seeing if the cloud exists is
	// so we can return an error if a cloud has been asked for that doesn't
	// exist. This is important as it will let us potentially identify bad logic
	// problems in Juju early where we have logic that might go off the rails
	// with bad values that make their way down to state.
	stmt := `
SELECT cloud_defaults.key,
       cloud_defaults.value,
       cloud.uuid
FROM cloud
LEFT JOIN cloud_defaults ON cloud.uuid = cloud_defaults.cloud_uuid
WHERE cloud.name = ?
`

	rval := make(map[string]string)
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, stmt, cloudName)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w %q", clouderrors.NotFound, cloudName)
		} else if err != nil {
			return fmt.Errorf("getting cloud %q defaults: %w", cloudName, err)
		}
		defer func() { _ = rows.Close() }()

		var (
			cloudUUID  string
			key, value sql.NullString
		)
		for rows.Next() {
			if err := rows.Scan(&key, &value, &cloudUUID); err != nil {
				return fmt.Errorf("reading cloud %q default: %w", cloudName, err)
			}
			if !key.Valid {
				// If the key is null it means there is no defaults set for the
				// cloud. We can safely just continue because the next iteration
				// of rows will return done.
				continue
			}
			rval[key.String] = value.String
		}

		if err := rows.Err(); err != nil {
			return fmt.Errorf("reading cloud %q defaults: %w", cloudName, err)
		}
		// If cloudUUID is the zero value it means no cloud exists for cloudName.
		if cloudUUID == "" {
			return fmt.Errorf("%w %q", clouderrors.NotFound, cloudName)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}
	return rval, nil
}

// UpdateCloudDefaults is responsible for updating default config values for a
// cloud. This function will allow the addition and updating of attributes.
// Attributes can also be removed by keys if they exist for the current cloud.
func (st *State) UpdateCloudDefaults(
	ctx context.Context,
	cloudName string,
	updateAttrs map[string]string,
	removeAttrs []string,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	selectStmt, err := st.Prepare("SELECT &Cloud.uuid FROM cloud WHERE name = $Cloud.name", Cloud{})
	if err != nil {
		return errors.Trace(err)
	}

	deleteStmt, err := st.Prepare(`
DELETE FROM  cloud_defaults
WHERE        key IN ($Attrs[:])
AND          cloud_uuid = $Cloud.uuid;
`, Attrs{}, Cloud{})
	if err != nil {
		return errors.Trace(err)
	}

	upsertStmt, err := sqlair.Prepare(`
INSERT INTO cloud_defaults (cloud_uuid, key, value) 
VALUES ($CloudDefaults.cloud_uuid, $CloudDefaults.key, $CloudDefaults.value)
ON CONFLICT(cloud_uuid, key) DO UPDATE
    SET value = excluded.value
    WHERE cloud_uuid = excluded.cloud_uuid
    AND key = excluded.key;
`, CloudDefaults{})
	if err != nil {
		return errors.Trace(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		cld := Cloud{Name: cloudName}
		err := tx.Query(ctx, selectStmt, cld).Get(&cld)
		if errors.Is(err, sqlair.ErrNoRows) {
			return fmt.Errorf("cloud %q %w%w", cloudName, errors.NotFound, errors.Hide(err))
		} else if err != nil {
			return fmt.Errorf("fetching cloud %q: %w", cloudName, domain.CoerceError(err))
		}

		if len(removeAttrs) > 0 {
			if err := tx.Query(ctx, deleteStmt, Attrs(removeAttrs), cld).Run(); err != nil {
				return fmt.Errorf("removing cloud default keys for %q: %w", cloudName, domain.CoerceError(err))
			}
		}

		for k, v := range updateAttrs {
			err := tx.Query(ctx, upsertStmt, CloudDefaults{ID: cld.ID, Key: k, Value: v}).Run()
			if database.IsErrConstraintNotNull(err) {
				return fmt.Errorf("missing cloud %q %w%w", cloudName, errors.NotValid, errors.Hide(err))
			} else if err != nil {
				return fmt.Errorf("updating cloud default keys %q: %w", cloudName, domain.CoerceError(err))
			}
		}

		return nil
	})
}

// CloudAllRegionDefaults returns all the default settings for a cloud and it's
// regions. Note this will not include the defaults set on the cloud itself but
// just that of it's regions. Empty map values are returned when no region
// defaults are found.
func (st *State) CloudAllRegionDefaults(
	ctx context.Context,
	cloudName string,
) (map[string]map[string]string, error) {
	defaults := map[string]map[string]string{}

	db, err := st.DB()
	if err != nil {
		return defaults, fmt.Errorf("getting database instance for cloud region defaults: %w", err)
	}

	stmt, err := st.Prepare(`
SELECT  (cloud_region.name,
        cloud_region_defaults.key,
        cloud_region_defaults.value)
		AS (&CloudRegionDefaultValue.*)
FROM    cloud_region_defaults
        INNER JOIN cloud_region
            ON cloud_region.uuid = cloud_region_defaults.region_uuid
        INNER JOIN cloud
            ON cloud_region.cloud_uuid = cloud.uuid
WHERE   cloud.name = $Cloud.name
`, CloudRegionDefaultValue{}, Cloud{})
	if err != nil {
		return defaults, errors.Trace(err)
	}

	return defaults, db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {

		var regionDefaultValues []CloudRegionDefaultValue

		if err := tx.Query(ctx, stmt, Cloud{Name: cloudName}).GetAll(&regionDefaultValues); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return nil
			}
			return fmt.Errorf("fetching cloud %q region defaults: %w", cloudName, domain.CoerceError(err))
		}

		for _, regionDefaultValue := range regionDefaultValues {
			store, has := defaults[regionDefaultValue.Name]
			if !has {
				store = map[string]string{}
				defaults[regionDefaultValue.Name] = store
			}
			store[regionDefaultValue.Key] = regionDefaultValue.Value
		}
		return nil
	})
}

// UpdateCloudRegionDefaults is responsible for updating default config values
// for a cloud region. This function will allow the addition and updating of
// attributes. Attributes can also be removed by keys if they exist for the
// current cloud. If the cloud or region is not found an error that satisfied
// NotValid is returned.
func (st *State) UpdateCloudRegionDefaults(
	ctx context.Context,
	cloudName string,
	regionName string,
	updateAttrs map[string]string,
	removeAttrs []string,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	selectStmt, err := st.Prepare(`
SELECT  cloud_region.uuid AS &CloudRegion.uuid
FROM    cloud_region
        INNER JOIN cloud
            ON cloud_region.cloud_uuid = cloud.uuid
WHERE   cloud.name = $Cloud.name
AND     cloud_region.name = $CloudRegion.name;
`, CloudRegion{}, Cloud{})
	if err != nil {
		return errors.Trace(err)
	}

	deleteStmt, err := st.Prepare(`
DELETE FROM  cloud_region_defaults
WHERE        key IN ($Attrs[:])
AND          region_uuid = $CloudRegion.uuid;
`, Attrs{}, CloudRegion{})
	if err != nil {
		return errors.Trace(err)
	}

	upsertStmt, err := st.Prepare(`
INSERT INTO cloud_region_defaults (region_uuid, key, value)
VALUES ($CloudRegionDefaults.region_uuid, $CloudRegionDefaults.key, $CloudRegionDefaults.value) 
ON CONFLICT(region_uuid, key) DO UPDATE
    SET value = excluded.value
    WHERE region_uuid = excluded.region_uuid
    AND key = excluded.key;
`, CloudRegionDefaults{})
	if err != nil {
		return errors.Trace(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		cloudRegion := CloudRegion{Name: regionName}
		if err := tx.Query(ctx, selectStmt, Cloud{Name: cloudName}, cloudRegion).Get(&cloudRegion); errors.Is(err, sqlair.ErrNoRows) {
			return fmt.Errorf("cloud %q region %q %w%w", cloudName, regionName, errors.NotFound, errors.Hide(err))
		} else if err != nil {
			return fmt.Errorf("fetching cloud %q region %q: %w", cloudName, regionName, domain.CoerceError(err))
		}

		if len(removeAttrs) > 0 {
			if err := tx.Query(ctx, deleteStmt, cloudRegion, Attrs(append(removeAttrs, cloudRegion.ID))).Run(); err != nil {
				return fmt.Errorf(
					"removing cloud %q region %q default keys: %w",
					cloudName,
					regionName,
					domain.CoerceError(err),
				)
			}
		}

		for k, v := range updateAttrs {
			err := tx.Query(ctx, upsertStmt, CloudRegionDefaults{ID: cloudRegion.ID, Key: k, Value: v}).Run()
			if database.IsErrConstraintNotNull(err) {
				return fmt.Errorf(
					"missing region %q for cloud %q %w%w",
					regionName,
					cloudName,
					errors.NotValid,
					errors.Hide(err),
				)
			} else if err != nil {
				return fmt.Errorf(
					"updating cloud %q region %q default keys: %w",
					cloudName,
					regionName,
					domain.CoerceError(err),
				)
			}
		}

		return nil
	})
}

// LoadClouds loads the cloud information from the database for the provided name.
func LoadClouds(ctx context.Context, st domain.Preparer, tx *sqlair.TX, name string) ([]cloud.Cloud, error) {
	q := `
	SELECT (uuid, name, cloud_type, cloud_type_id, endpoint,
            identity_endpoint, storage_endpoint, skip_tls_verify,
            is_controller_cloud) AS (&Cloud.*),
            auth_type AS &M.cloud_auth_type
    FROM v_cloud_auth
`

	args := []any{}
	if name != "" {
		q += "WHERE name = $M.cloud_name"
		args = append(args, sqlair.M{
			"cloud_name": name,
		})
	}

	loadCloudStmt, err := st.Prepare(q, sqlair.M{}, Cloud{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	iter := tx.Query(ctx, loadCloudStmt, args...).Iter()
	defer func() { _ = iter.Close() }()

	clouds := make(map[string]*cloud.Cloud)
	m := sqlair.M{}
	for iter.Next() {
		var dbCloud Cloud
		if err := iter.Get(&dbCloud, m); err != nil {
			return nil, errors.Trace(err)
		}
		cld, ok := clouds[dbCloud.ID]
		if !ok {
			cld = &cloud.Cloud{
				Name:              dbCloud.Name,
				Type:              dbCloud.Type,
				Endpoint:          dbCloud.Endpoint,
				IdentityEndpoint:  dbCloud.IdentityEndpoint,
				StorageEndpoint:   dbCloud.StorageEndpoint,
				SkipTLSVerify:     dbCloud.SkipTLSVerify,
				IsControllerCloud: dbCloud.IsControllerCloud,
				// These are filled in below.
				AuthTypes:      nil,
				Regions:        nil,
				CACertificates: nil,
			}
			clouds[dbCloud.ID] = cld
		}
		// "cloud_auth_type" will be in the map since iter.Get succeeded but may be set to nil.
		if cloudAuthType, ok := m["cloud_auth_type"]; !ok {
			return nil, fmt.Errorf("error getting cloud type from database")
		} else if cloudAuthType != nil {
			cld.AuthTypes = append(cld.AuthTypes, cloud.AuthType(cloudAuthType.(string)))
		}
	}
	if err := iter.Close(); err != nil {
		return nil, errors.Trace(domain.CoerceError(err))
	}

	var uuids []string
	for uuid := range clouds {
		uuids = append(uuids, uuid)
	}

	// Add in the ca certs and regions.
	caCerts, err := loadCACerts(ctx, tx, uuids)
	if err != nil {
		return nil, errors.Trace(err)
	}
	for uuid, certs := range caCerts {
		clouds[uuid].CACertificates = certs
	}

	cloudRegions, err := loadRegions(ctx, tx, uuids)
	if err != nil {
		return nil, errors.Trace(err)
	}
	for uuid, regions := range cloudRegions {
		clouds[uuid].Regions = regions
	}

	var result []cloud.Cloud
	for _, c := range clouds {
		result = append(result, *c)
	}
	return result, nil
}

// loadCACerts loads the ca certs for the specified clouds, returning
// a map of results keyed on cloud uuid.
func loadCACerts(ctx context.Context, tx *sqlair.TX, cloudUUIDs []string) (map[string][]string, error) {
	loadCACertStmt, err := sqlair.Prepare(`
SELECT &CloudCACert.*
FROM   cloud_ca_cert
WHERE  cloud_uuid IN ($CloudUUIDs[:])
`, CloudUUIDs{}, CloudCACert{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	var dbCloudCACerts []CloudCACert
	err = tx.Query(ctx, loadCACertStmt, CloudUUIDs(cloudUUIDs)).GetAll(&dbCloudCACerts)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Trace(domain.CoerceError(err))
	}

	var result = map[string][]string{}
	for _, dbCloudCACert := range dbCloudCACerts {
		cloudUUID := dbCloudCACert.CloudUUID
		_, ok := result[cloudUUID]
		if !ok {
			result[cloudUUID] = []string{}
		}
		result[cloudUUID] = append(result[cloudUUID], dbCloudCACert.CACert)
	}
	return result, nil
}

// loadRegions loads the regions for the specified clouds, returning
// a map of results keyed on cloud uuid.
func loadRegions(ctx context.Context, tx *sqlair.TX, cloudUUIDS []string) (map[string][]cloud.Region, error) {
	loadRegionsStmt, err := sqlair.Prepare(`
SELECT &CloudRegion.*
FROM   cloud_region
WHERE  cloud_uuid IN ($CloudUUIDs[:])
`[1:], CloudUUIDs{}, CloudRegion{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	var dbRegions []CloudRegion
	err = tx.Query(ctx, loadRegionsStmt, CloudUUIDs(cloudUUIDS)).GetAll(&dbRegions)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Trace(domain.CoerceError(err))
	}

	var result = map[string][]cloud.Region{}
	for _, dbRegion := range dbRegions {
		cloudUUID := dbRegion.CloudUUID
		_, ok := result[cloudUUID]
		if !ok {
			result[cloudUUID] = []cloud.Region{}
		}
		result[cloudUUID] = append(result[cloudUUID], cloud.Region{
			Name:             dbRegion.Name,
			Endpoint:         dbRegion.Endpoint,
			IdentityEndpoint: dbRegion.IdentityEndpoint,
			StorageEndpoint:  dbRegion.StorageEndpoint,
		})
	}
	return result, nil
}

// UpdateCloud updates the specified cloud.
func (st *State) UpdateCloud(ctx context.Context, cloud cloud.Cloud) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	selectUUIDStmt, err := st.Prepare("SELECT &Cloud.uuid FROM cloud WHERE name = $Cloud.name", Cloud{})
	if err != nil {
		return errors.Trace(domain.CoerceError(err))
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Get the cloud UUID
		dbCloud := Cloud{Name: cloud.Name}
		err := tx.Query(ctx, selectUUIDStmt, dbCloud).Get(&dbCloud)
		if err != nil && errors.Is(err, sqlair.ErrNoRows) {
			return fmt.Errorf("%q %w", cloud.Name, clouderrors.NotFound)
		} else if err != nil {
			return errors.Trace(domain.CoerceError(err))
		}
		cloudUUID := dbCloud.ID

		if err := updateCloud(ctx, tx, cloudUUID, cloud); err != nil {
			return errors.Annotate(err, "updating cloud regions")
		}
		return nil
	})

	return errors.Trace(err)
}

// CreateCloud creates a cloud and provides admin permissions to the
// provided ownerName.
// This is the exported method for use with the cloud state.
func (st *State) CreateCloud(ctx context.Context, ownerName, cloudUUID string, cloud cloud.Cloud) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return CreateCloud(ctx, tx, ownerName, cloudUUID, cloud)
	})
	return errors.Trace(err)
}

// CreateCloud saves the specified cloud and creates Admin permission on the
// cloud for the provided user.
// Exported for use in the related cloud bootstrap package.
// Should never be directly called outside of the cloud bootstrap package.
func CreateCloud(ctx context.Context, tx *sqlair.TX, ownerName, cloudUUID string, cloud cloud.Cloud) error {
	if err := updateCloud(ctx, tx, cloudUUID, cloud); err != nil {
		return errors.Annotatef(err, "updating cloud %s", cloudUUID)
	}
	if err := insertPermission(ctx, tx, ownerName, cloud.Name); err != nil {
		return errors.Annotate(err, "inserting cloud user permission")
	}
	return nil
}

func updateCloud(ctx context.Context, tx *sqlair.TX, cloudUUID string, cloud cloud.Cloud) error {
	if err := upsertCloud(ctx, tx, cloudUUID, cloud); err != nil {
		return errors.Annotatef(err, "updating cloud %s", cloudUUID)
	}
	if err := updateAuthTypes(ctx, tx, cloudUUID, cloud.AuthTypes); err != nil {
		return errors.Annotatef(err, "updating cloud %s auth types", cloudUUID)
	}
	if err := updateCACerts(ctx, tx, cloudUUID, cloud.CACertificates); err != nil {
		return errors.Annotatef(err, "updating cloud %s CA certs", cloudUUID)
	}
	if err := updateRegions(ctx, tx, cloudUUID, cloud.Regions); err != nil {
		return errors.Annotatef(err, "updating cloud %s regions", cloudUUID)
	}
	return nil
}

func upsertCloud(ctx context.Context, tx *sqlair.TX, cloudUUID string, cloud cloud.Cloud) error {
	dbCloud, err := dbCloudFromCloud(ctx, tx, cloudUUID, cloud)
	if err != nil {
		return errors.Trace(err)
	}

	insertCloudStmt, err := sqlair.Prepare(`
INSERT INTO cloud (uuid, name, cloud_type_id, endpoint,
                   identity_endpoint, storage_endpoint,
                   skip_tls_verify)
VALUES ($Cloud.uuid, $Cloud.name, $Cloud.cloud_type_id, $Cloud.endpoint, 
        $Cloud.identity_endpoint, $Cloud.storage_endpoint,
        $Cloud.skip_tls_verify)
ON CONFLICT(uuid) DO UPDATE SET name=excluded.name,
                                endpoint=excluded.endpoint,
                                identity_endpoint=excluded.identity_endpoint,
                                storage_endpoint=excluded.storage_endpoint,
                                skip_tls_verify=excluded.skip_tls_verify;
`, Cloud{})
	if err != nil {
		return errors.Trace(err)
	}

	err = tx.Query(ctx, insertCloudStmt, dbCloud).Run()
	if database.IsErrConstraintCheck(err) {
		return fmt.Errorf("%w cloud name cannot be empty%w", errors.NotValid, errors.Hide(err))
	} else if err != nil {
		return errors.Trace(domain.CoerceError(err))
	}
	return nil
}

// loadAuthTypes reads the cloud auth type values and ids
// into a map for easy lookup.
func loadAuthTypes(ctx context.Context, tx *sqlair.TX) (map[string]int, error) {
	var dbAuthTypes = map[string]int{}

	stmt, err := sqlair.Prepare("SELECT &AuthType.* FROM auth_type", AuthType{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	var authTypes []AuthType
	err = tx.Query(ctx, stmt).GetAll(&authTypes)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Trace(domain.CoerceError(err))
	}
	for _, authType := range authTypes {
		dbAuthTypes[authType.Type] = authType.ID
	}
	return dbAuthTypes, nil
}

func updateAuthTypes(ctx context.Context, tx *sqlair.TX, cloudUUID string, authTypes cloud.AuthTypes) error {
	dbAuthTypes, err := loadAuthTypes(ctx, tx)
	if err != nil {
		return errors.Trace(err)
	}

	// First validate the passed in auth types.
	var authTypeIds = make(AuthTypeIds, len(authTypes))
	for i, a := range authTypes {
		id, ok := dbAuthTypes[string(a)]
		if !ok {
			return errors.NotValidf("auth type %q", a)
		}
		authTypeIds[i] = id
	}

	// Delete auth types no longer in the list.
	deleteQuery, err := sqlair.Prepare(`
DELETE FROM  cloud_auth_type
WHERE        cloud_uuid = $M.cloud_uuid
AND          auth_type_id NOT IN ($AuthTypeIds[:])
`, authTypeIds, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}

	if err := tx.Query(ctx, deleteQuery, authTypeIds, sqlair.M{"cloud_uuid": cloudUUID}).Run(); err != nil {
		return errors.Trace(domain.CoerceError(err))
	}

	insertStmt, err := sqlair.Prepare(`
INSERT INTO cloud_auth_type (cloud_uuid, auth_type_id)
VALUES ($CloudAuthType.cloud_uuid, $CloudAuthType.auth_type_id)
ON CONFLICT(cloud_uuid, auth_type_id) DO NOTHING;
	`, CloudAuthType{})
	if err != nil {
		return errors.Trace(err)
	}

	for _, a := range authTypeIds {
		cloudAuthType := CloudAuthType{CloudUUID: cloudUUID, AuthTypeID: a}
		if err := tx.Query(ctx, insertStmt, cloudAuthType).Run(); err != nil {
			return errors.Trace(domain.CoerceError(err))
		}
	}
	return nil
}

func updateCACerts(ctx context.Context, tx *sqlair.TX, cloudUUID string, certs []string) error {
	// Delete any existing ca certs - we just delete them all rather
	// than keeping existing ones as the cert values are long strings.
	deleteQuery, err := sqlair.Prepare(`
DELETE FROM  cloud_ca_cert
WHERE        cloud_uuid = $M.cloud_uuid
`, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}
	insertQuery, err := sqlair.Prepare(`
INSERT INTO cloud_ca_cert (cloud_uuid, ca_cert)
VALUES ($CloudCACert.cloud_uuid, $CloudCACert.ca_cert)
`, CloudCACert{})
	if err != nil {
		return errors.Trace(err)
	}

	if err := tx.Query(ctx, deleteQuery, sqlair.M{"cloud_uuid": cloudUUID}).Run(); err != nil {
		return errors.Trace(err)
	}

	for _, cert := range certs {
		cloudCACert := CloudCACert{CloudUUID: cloudUUID, CACert: cert}
		if err := tx.Query(ctx, insertQuery, cloudCACert).Run(); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func updateRegions(ctx context.Context, tx *sqlair.TX, cloudUUID string, regions []cloud.Region) error {
	regionNames := RegionNames(transform.Slice(regions, func(r cloud.Region) string { return r.Name }))

	deleteQuery, err := sqlair.Prepare(`
DELETE FROM  cloud_region
WHERE        cloud_uuid = $M.cloud_uuid
AND          name NOT IN ($RegionNames[:])
`, RegionNames{}, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}

	insertQuery, err := sqlair.Prepare(`
INSERT INTO cloud_region (uuid, cloud_uuid, name,
                          endpoint, identity_endpoint,
                          storage_endpoint)
VALUES ($CloudRegion.uuid, $CloudRegion.cloud_uuid, $CloudRegion.name, 
        $CloudRegion.endpoint, $CloudRegion.identity_endpoint, 
        $CloudRegion.storage_endpoint)
ON CONFLICT(cloud_uuid, name) DO UPDATE SET name=excluded.name,
                                            endpoint=excluded.endpoint,
                                            identity_endpoint=excluded.identity_endpoint,
                                            storage_endpoint=excluded.storage_endpoint
`, CloudRegion{})
	if err != nil {
		return errors.Trace(err)
	}

	// Delete any regions no longer in the list.
	if err := tx.Query(ctx, deleteQuery, sqlair.M{"cloud_uuid": cloudUUID}, regionNames).Run(); err != nil {
		return errors.Trace(err)
	}

	for _, r := range regions {
		cloudRegion := CloudRegion{ID: uuid.MustNewUUID().String(),
			CloudUUID: cloudUUID, Name: r.Name, Endpoint: r.Endpoint,
			IdentityEndpoint: r.IdentityEndpoint,
			StorageEndpoint:  r.StorageEndpoint}
		if err := tx.Query(ctx, insertQuery, cloudRegion).Run(); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// insertPermission inserts a permission for the owner of the cloud during
// upsertCloud.
func insertPermission(ctx context.Context, tx *sqlair.TX, ownerName, cloudName string) error {
	if ownerName == "" {
		return nil
	}
	newPermission := `
INSERT INTO permission (uuid, access_type_id, object_type_id, grant_to, grant_on)
SELECT $dbAddUserPermission.uuid,
       at.id,
       ot.id,
       u.uuid,
       $dbAddUserPermission.grant_on
FROM   v_user_auth u,
       permission_access_type at,
       permission_object_type ot
WHERE  u.name = $dbAddUserPermission.name
AND    u.disabled = false
AND    u.removed = false
AND    at.type = $dbAddUserPermission.access_type
AND    ot.type = $dbAddUserPermission.object_type
`
	insertPermissionStmt, err := sqlair.Prepare(newPermission, dbAddUserPermission{})
	if err != nil {
		return errors.Trace(err)
	}

	permUUID, err := uuid.NewUUID()
	if err != nil {
		return errors.Trace(err)
	}
	perm := dbAddUserPermission{
		UUID:       permUUID.String(),
		GrantOn:    cloudName,
		Name:       ownerName,
		AccessType: string(permission.AdminAccess),
		ObjectType: string(permission.Cloud),
	}

	err = tx.Query(ctx, insertPermissionStmt, perm).Run()
	if err != nil && database.IsErrConstraintUnique(err) {
		return fmt.Errorf("for %q on %q, %w", ownerName, cloudName, accesserrors.PermissionAlreadyExists)
	} else if err != nil && (database.IsErrConstraintForeignKey(err) || errors.Is(err, sqlair.ErrNoRows)) {
		return fmt.Errorf("%q %w", ownerName, accesserrors.UserNotFound)
	} else if err != nil {
		return errors.Annotatef(domain.CoerceError(err), "adding permission %q for %q on %q", string(permission.AdminAccess), ownerName, cloudName)
	}

	return nil
}

func dbCloudFromCloud(ctx context.Context, tx *sqlair.TX, cloudUUID string, cloud cloud.Cloud) (*Cloud, error) {
	cld := &Cloud{
		ID:                cloudUUID,
		Name:              cloud.Name,
		Type:              cloud.Type,
		Endpoint:          cloud.Endpoint,
		IdentityEndpoint:  cloud.IdentityEndpoint,
		StorageEndpoint:   cloud.StorageEndpoint,
		SkipTLSVerify:     cloud.SkipTLSVerify,
		IsControllerCloud: cloud.IsControllerCloud,
	}

	selectCloudIDstmt, err := sqlair.Prepare("SELECT id AS &Cloud.cloud_type_id FROM cloud_type WHERE type = $CloudType.type", Cloud{}, CloudType{})
	if err != nil {
		return nil, errors.Trace(err)
	}
	cloudType := CloudType{Type: cloud.Type}
	err = tx.Query(ctx, selectCloudIDstmt, cloudType).Get(cld)
	if errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.NotValidf("cloud type %q", cloud.Type)
	}
	if err != nil {
		return nil, errors.Trace(domain.CoerceError(err))
	}
	return cld, nil
}

// DeleteCloud removes a cloud credential with the given name.
func (st *State) DeleteCloud(ctx context.Context, name string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	// TODO(wallyworld) - also check model reference
	cloudDeleteQ := `
DELETE FROM cloud
WHERE  cloud.name = ?
AND cloud.uuid NOT IN (
    SELECT cloud_uuid FROM cloud_credential
)
`

	cloudRegionDeleteQ := `
DELETE FROM cloud_region
    WHERE cloud_uuid IN (
        SELECT uuid FROM cloud WHERE cloud.name = ?
    )
`

	cloudCACertDeleteQ := `
DELETE FROM cloud_ca_cert
    WHERE cloud_uuid IN (
        SELECT uuid FROM cloud WHERE cloud.name = ?
    )
`

	cloudAuthTypeDeleteQ := `
DELETE FROM cloud_auth_type
    WHERE cloud_uuid IN (
        SELECT uuid FROM cloud WHERE cloud.name = ?
    )
`

	permissionsQ := `
DELETE FROM permission
WHERE  grant_on = ?
`

	return db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, cloudRegionDeleteQ, name)
		if err != nil {
			return errors.Annotate(err, "deleting cloud regions")
		}
		_, err = tx.ExecContext(ctx, cloudCACertDeleteQ, name)
		if err != nil {
			return errors.Annotate(err, "deleting cloud ca certs")
		}
		_, err = tx.ExecContext(ctx, cloudAuthTypeDeleteQ, name)
		if err != nil {
			return errors.Annotate(err, "deleting cloud auth type")
		}
		_, err = tx.ExecContext(ctx, permissionsQ, name)
		if err != nil {
			return errors.Annotate(err, "deleting permissions on cloud")
		}
		result, err := tx.ExecContext(ctx, cloudDeleteQ, name)
		if err != nil {
			return errors.Annotate(err, "deleting cloud")
		}
		num, err := result.RowsAffected()
		if err != nil {
			return errors.Trace(err)
		}
		if num == 0 {
			return errors.Errorf("cannot delete cloud as it is still in use")
		}
		return nil
	})
}

// AllowCloudType is responsible for applying the cloud type to
// the given database. If the unique constraint applies the error is masked and
// returned as NIL.
func AllowCloudType(ctx context.Context, db coredatabase.TxnRunner, version int, name string) error {
	return errors.Trace(db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO cloud_type VALUES (?, ?)`, version, name)
		if database.IsErrConstraintUnique(err) {
			return nil
		}
		return err
	}))
}

// WatchCloud returns a new NotifyWatcher watching for changes to the specified cloud.
func (st *State) WatchCloud(
	ctx context.Context,
	getWatcher func(string, string, changestream.ChangeType) (watcher.NotifyWatcher, error),
	cloudName string,
) (watcher.NotifyWatcher, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var uuid string
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, "SELECT uuid FROM cloud WHERE name = ?", cloudName)
		if err := row.Scan(&uuid); errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("cloud %q %w%w", cloudName, errors.NotFound, errors.Hide(err))
		} else if err != nil {
			return fmt.Errorf("fetching cloud %q: %w", cloudName, domain.CoerceError(err))
		}
		return nil
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	result, err := getWatcher("cloud", uuid, changestream.All)
	return result, errors.Annotatef(err, "watching cloud")
}

// SetCloudDefaults is responsible for removing any previously set cloud
// default values and setting the new cloud defaults to use. If no defaults are
// supplied to this function then the currently set cloud default values will be
// removed and no further operations will be performed. If no cloud exists for
// the cloud name then an error satisfying [clouderrors.NotFound] is returned.
func SetCloudDefaults(
	ctx context.Context,
	tx *sql.Tx,
	cloudName string,
	defaults map[string]string,
) error {
	cloudUUIDStmt := "SELECT uuid FROM cloud WHERE name = ?"

	var cloudUUID string
	row := tx.QueryRowContext(ctx, cloudUUIDStmt, cloudName)
	err := row.Scan(&cloudUUID)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w %q", clouderrors.NotFound, cloudName)
	} else if err != nil {
		return fmt.Errorf("getting cloud %q uuid to set cloud model defaults: %w", cloudName, err)
	}

	deleteStmt := "DELETE FROM cloud_defaults WHERE cloud_defaults.cloud_uuid = ?"
	_, err = tx.ExecContext(ctx, deleteStmt, cloudUUID)
	if err != nil {
		return fmt.Errorf("removing previously set cloud %q model defaults: %w", cloudName, err)
	}

	if len(defaults) == 0 {
		return nil
	}

	bindStr, args := database.MapToMultiPlaceholderTransform(defaults, func(k, v string) []any {
		return []any{cloudUUID, k, v}
	})

	insertStmt := fmt.Sprintf(
		"INSERT INTO cloud_defaults (cloud_uuid, key, value) VALUES %s",
		bindStr,
	)

	_, err = tx.ExecContext(ctx, insertStmt, args...)
	if err != nil {
		return fmt.Errorf("setting cloud %q model defaults: %w", cloudName, err)
	}

	return nil
}
