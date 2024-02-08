// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	stderrors "errors"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"
	"github.com/juju/utils/v3"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/changestream"
	coredatabase "github.com/juju/juju/core/database"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain"
	clouderrors "github.com/juju/juju/domain/cloud/errors"
	"github.com/juju/juju/internal/database"
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
func (st *State) ListClouds(ctx context.Context, name string) ([]cloud.Cloud, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var result []cloud.Cloud
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		result, err = loadClouds(ctx, tx, name)
		return errors.Trace(err)
	})
	return result, errors.Trace(err)
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

	selectStmt := "SELECT uuid FROM cloud WHERE name = ?"

	deleteBinds, deleteVals := database.SliceToPlaceholder(removeAttrs)
	deleteStmt := fmt.Sprintf(`
DELETE FROM  cloud_defaults
WHERE        key IN (%s)
AND          cloud_uuid = ?;
`, deleteBinds)

	upsertStmt := fmt.Sprintf(`
INSERT INTO cloud_defaults (cloud_uuid, key, value) 
VALUES %s 
ON CONFLICT(cloud_uuid, key) DO UPDATE
    SET value = excluded.value
    WHERE cloud_uuid = excluded.cloud_uuid
    AND key = excluded.key;
`, database.MakeBindArgs(3, len(updateAttrs)))

	return db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		var uuid string
		row := tx.QueryRowContext(ctx, selectStmt, cloudName)
		if err := row.Scan(&uuid); err == sql.ErrNoRows {
			return fmt.Errorf("cloud %q %w%w", cloudName, errors.NotFound, errors.Hide(err))
		} else if err != nil {
			return fmt.Errorf("fetching cloud %q: %w", cloudName, err)
		}

		if len(deleteVals) > 0 {
			_, err := tx.ExecContext(ctx, deleteStmt, append(deleteVals, uuid)...)
			if err != nil {
				return fmt.Errorf("removing cloud default keys for %q: %w", cloudName, err)
			}
		}

		if len(updateAttrs) > 0 {
			values := make([]any, 0, len(updateAttrs)*3)
			for k, v := range updateAttrs {
				values = append(values, uuid, k, v)
			}
			_, err := tx.ExecContext(ctx, upsertStmt, values...)
			if database.IsErrConstraintNotNull(err) {
				return fmt.Errorf("missing cloud %q %w%w", cloudName, errors.NotValid, errors.Hide(err))
			} else if err != nil {
				return fmt.Errorf("updating cloud default keys %q: %w", cloudName, err)
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

	stmt := `
SELECT  cloud_region.name,
        cloud_region_defaults.key,
        cloud_region_defaults.value
FROM    cloud_region_defaults
        INNER JOIN cloud_region
            ON cloud_region.uuid = cloud_region_defaults.region_uuid
        INNER JOIN cloud
            ON cloud_region.cloud_uuid = cloud.uuid
WHERE   cloud.name = ?
`

	return defaults, db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, stmt, cloudName)
		if err != nil {
			return fmt.Errorf("fetching cloud %q region defaults: %w", cloudName, err)
		}
		defer rows.Close()

		var regionName, key, value string
		for rows.Next() {
			if err := rows.Scan(&regionName, &key, &value); err != nil {
				return fmt.Errorf(
					"compiling cloud %q region %q defaults: %w",
					cloudName,
					regionName,
					stderrors.Join(err, rows.Err()),
				)
			}
			store, has := defaults[regionName]
			if !has {
				store = map[string]string{}
				defaults[regionName] = store
			}
			store[key] = value
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

	selectStmt := `
SELECT  cloud_region.uuid
FROM    cloud_region
        INNER JOIN cloud
            ON cloud_region.cloud_uuid = cloud.uuid
WHERE   cloud.name = ?
AND     cloud_region.name = ?;
`

	deleteBinds, deleteVals := database.SliceToPlaceholder(removeAttrs)
	deleteStmt := fmt.Sprintf(`
DELETE FROM  cloud_region_defaults
WHERE        key IN (%s)
AND          region_uuid = ?;
`, deleteBinds)

	upsertStmt := fmt.Sprintf(`
INSERT INTO cloud_region_defaults (region_uuid, key, value)
VALUES %s
ON CONFLICT(region_uuid, key) DO UPDATE
    SET value = excluded.value
    WHERE region_uuid = excluded.region_uuid
    AND key = excluded.key;
`, database.MakeBindArgs(3, len(updateAttrs)))

	return db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		var uuid string
		row := tx.QueryRowContext(ctx, selectStmt, cloudName, regionName)
		if err := row.Scan(&uuid); err == sql.ErrNoRows {
			return fmt.Errorf("cloud %q region %q %w%w", cloudName, regionName, errors.NotFound, errors.Hide(err))
		} else if err != nil {
			return fmt.Errorf("fetching cloud %q region %q: %w", cloudName, regionName, err)
		}

		if len(deleteVals) > 0 {
			_, err := tx.ExecContext(ctx, deleteStmt, append(deleteVals, uuid)...)
			if err != nil {
				return fmt.Errorf(
					"removing cloud %q region %q default keys: %w",
					cloudName,
					regionName,
					err,
				)
			}
		}

		if len(updateAttrs) > 0 {
			values := make([]any, 0, len(updateAttrs)*3)
			for k, v := range updateAttrs {
				values = append(values, uuid, k, v)
			}
			_, err := tx.ExecContext(ctx, upsertStmt, values...)
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
					err,
				)
			}
		}

		return nil
	})
}

func loadClouds(ctx context.Context, tx *sql.Tx, name string) ([]cloud.Cloud, error) {
	// First load the basic cloud info and auth types.
	q := `
SELECT cloud.uuid, cloud.name, cloud_type_id, 
       cloud.endpoint, cloud.identity_endpoint, 
       cloud.storage_endpoint, skip_tls_verify, 
       auth_type.type, cloud_type.type,
       COALESCE((SELECT true
        FROM model_metadata
		INNER JOIN user
        ON user.uuid = model_metadata.owner_uuid
        WHERE cloud_uuid = cloud.uuid
        AND model_metadata.name = ?
        AND user.name = ?), false) AS controller_cloud
FROM   cloud
       LEFT JOIN cloud_auth_type 
            ON cloud.uuid = cloud_auth_type.cloud_uuid
       JOIN auth_type 
            ON auth_type.id = cloud_auth_type.auth_type_id
       JOIN cloud_type 
            ON cloud_type.id = cloud.cloud_type_id
`

	args := []any{
		coremodel.ControllerModelName,
		coremodel.ControllerModelOwnerUsername,
	}
	if name != "" {
		q = q + "WHERE cloud.name = ?"
		args = append(args, name)
	}

	rows, err := tx.QueryContext(ctx, q, args...)
	if err != nil && err != sql.ErrNoRows {
		return nil, errors.Trace(err)
	}
	defer func() { _ = rows.Close() }()

	clouds := make(map[string]*cloud.Cloud)
	for rows.Next() {
		var (
			dbCloud       Cloud
			cloudType     string
			cloudAuthType string
		)
		if err := rows.Scan(
			&dbCloud.ID, &dbCloud.Name, &dbCloud.TypeID, &dbCloud.Endpoint, &dbCloud.IdentityEndpoint,
			&dbCloud.StorageEndpoint, &dbCloud.SkipTLSVerify, &cloudAuthType, &cloudType, &dbCloud.IsControllerCloud,
		); err != nil {
			return nil, errors.Trace(err)
		}
		cld, ok := clouds[dbCloud.ID]
		if !ok {
			cld = &cloud.Cloud{
				Name:              dbCloud.Name,
				Type:              cloudType,
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
		if cloudAuthType != "" {
			cld.AuthTypes = append(cld.AuthTypes, cloud.AuthType(cloudAuthType))
		}
	}
	if err := rows.Err(); err != nil {
		return nil, errors.Trace(err)
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
func loadCACerts(ctx context.Context, tx *sql.Tx, cloudUUIDS []string) (map[string][]string, error) {
	cloudUUIDBinds, cloudUUIDsVals := database.SliceToPlaceholder(cloudUUIDS)
	q := fmt.Sprintf(`
SELECT cloud_uuid, ca_cert
FROM   cloud_ca_cert
WHERE  cloud_uuid IN (%s)
`, cloudUUIDBinds)

	rows, err := tx.QueryContext(ctx, q, cloudUUIDsVals...)
	if err != nil && err != sql.ErrNoRows {
		return nil, errors.Trace(err)
	}
	defer func() { _ = rows.Close() }()

	var result = map[string][]string{}
	for rows.Next() {
		var (
			cloudUUID string
			cert      string
		)
		if err := rows.Scan(&cloudUUID, &cert); err != nil {
			return nil, errors.Trace(err)
		}
		_, ok := result[cloudUUID]
		if !ok {
			result[cloudUUID] = []string{}
		}
		result[cloudUUID] = append(result[cloudUUID], cert)
	}
	return result, rows.Err()
}

// loadRegions loads the regions for the specified clouds, returning
// a map of results keyed on cloud uuid.
func loadRegions(ctx context.Context, tx *sql.Tx, cloudUUIDS []string) (map[string][]cloud.Region, error) {
	cloudUUIDBinds, cloudUUIDSAnyVals := database.SliceToPlaceholder(cloudUUIDS)
	q := fmt.Sprintf(`
SELECT cloud_uuid, name, endpoint, identity_endpoint, storage_endpoint
FROM   cloud_region
WHERE  cloud_uuid IN (%s)
		`, cloudUUIDBinds)[1:]

	rows, err := tx.QueryContext(ctx, q, cloudUUIDSAnyVals...)
	if err != nil && err != sql.ErrNoRows {
		return nil, errors.Trace(err)
	}
	defer func() { _ = rows.Close() }()

	var result = map[string][]cloud.Region{}
	for rows.Next() {
		var (
			cloudUUID string
			dbRegion  CloudRegion
		)
		if err := rows.Scan(&cloudUUID, &dbRegion.Name, &dbRegion.Endpoint, &dbRegion.IdentityEndpoint, &dbRegion.StorageEndpoint); err != nil {
			return nil, errors.Trace(err)
		}
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
	return result, rows.Err()
}

// UpsertCloud inserts or updates the specified cloud.
func (st *State) UpsertCloud(ctx context.Context, cloud cloud.Cloud) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		// Get the cloud UUID - either existing or make a new one.
		var cloudUUID string
		row := tx.QueryRowContext(ctx, "SELECT uuid FROM cloud WHERE name = ?", cloud.Name)
		err := row.Scan(&cloudUUID)
		if err != nil && err != sql.ErrNoRows {
			return errors.Trace(err)
		}
		if err != nil {
			cloudUUID = utils.MustNewUUID().String()
		}

		if err := upsertCloud(ctx, tx, cloudUUID, cloud); err != nil {
			return errors.Annotate(err, "updating cloud")
		}

		if err := updateAuthTypes(ctx, tx, cloudUUID, cloud.AuthTypes); err != nil {
			return errors.Annotate(err, "updating cloud auth types")
		}

		if err := updateCACerts(ctx, tx, cloudUUID, cloud.CACertificates); err != nil {
			return errors.Annotate(err, "updating cloud CA certs")
		}

		if err := updateRegions(ctx, tx, cloudUUID, cloud.Regions); err != nil {
			return errors.Annotate(err, "updating cloud regions")
		}

		return nil
	})

	return errors.Trace(err)
}

// CreateCloud saves the specified cloud.
// Exported for use in the related cloud bootstrap package.
func CreateCloud(ctx context.Context, tx *sql.Tx, cloudUUID string, cloud cloud.Cloud) error {
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

func upsertCloud(ctx context.Context, tx *sql.Tx, cloudUUID string, cloud cloud.Cloud) error {
	dbCloud, err := dbCloudFromCloud(ctx, tx, cloudUUID, cloud)
	if err != nil {
		return errors.Trace(err)
	}

	q := `
INSERT INTO cloud (uuid, name, cloud_type_id, endpoint, identity_endpoint, storage_endpoint, skip_tls_verify)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(uuid) DO UPDATE SET name=excluded.name,
                                endpoint=excluded.endpoint,
                                identity_endpoint=excluded.identity_endpoint,
                                storage_endpoint=excluded.storage_endpoint,
                                skip_tls_verify=excluded.skip_tls_verify;`

	_, err = tx.ExecContext(ctx, q,
		dbCloud.ID,
		dbCloud.Name,
		dbCloud.TypeID,
		dbCloud.Endpoint,
		dbCloud.IdentityEndpoint,
		dbCloud.StorageEndpoint,
		dbCloud.SkipTLSVerify,
	)
	if database.IsErrConstraintCheck(err) {
		return fmt.Errorf("%w cloud name cannot be empty%w", errors.NotValid, errors.Hide(err))
	} else if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// loadAuthTypes reads the cloud auth type values and ids
// into a map for easy lookup.
func loadAuthTypes(ctx context.Context, tx *sql.Tx) (map[string]int, error) {
	var dbAuthTypes = map[string]int{}

	rows, err := tx.QueryContext(ctx, "SELECT id, type FROM auth_type")
	if err != nil && err != sql.ErrNoRows {
		return nil, errors.Trace(err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var (
			id    int
			value string
		)
		if err := rows.Scan(&id, &value); err != nil {
			return nil, errors.Trace(err)
		}
		dbAuthTypes[value] = id
	}
	return dbAuthTypes, rows.Err()
}

func updateAuthTypes(ctx context.Context, tx *sql.Tx, cloudUUID string, authTypes cloud.AuthTypes) error {
	dbAuthTypes, err := loadAuthTypes(ctx, tx)
	if err != nil {
		return errors.Trace(err)
	}

	// First validate the passed in auth types.
	var authTypeIds = make([]int, len(authTypes))
	for i, a := range authTypes {
		id, ok := dbAuthTypes[string(a)]
		if !ok {
			return errors.NotValidf("auth type %q", a)
		}
		authTypeIds[i] = id
	}

	authTypeIdsBinds, authTypeIdsAnyVals := database.SliceToPlaceholder(authTypeIds)

	// Delete auth types no longer in the list.
	deleteQuery := fmt.Sprintf(`
DELETE FROM  cloud_auth_type
WHERE        cloud_uuid = ?
AND          auth_type_id NOT IN (%s)
`, authTypeIdsBinds)

	args := append([]any{cloudUUID}, authTypeIdsAnyVals...)
	if _, err := tx.ExecContext(ctx, deleteQuery, args...); err != nil {
		return errors.Trace(err)
	}

	insertQuery := `
INSERT INTO cloud_auth_type (uuid, cloud_uuid, auth_type_id)
VALUES (?, ?, ?)
ON CONFLICT(cloud_uuid, auth_type_id) DO NOTHING;
	`
	for _, a := range authTypeIds {
		if _, err := tx.ExecContext(ctx, insertQuery, utils.MustNewUUID().String(), cloudUUID, a); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func updateCACerts(ctx context.Context, tx *sql.Tx, cloudUUID string, certs []string) error {
	// Delete any existing ca certs - we just delete them all rather
	// than keeping existing ones as the cert values are long strings.
	deleteQuery := `
DELETE FROM  cloud_ca_cert
WHERE        cloud_uuid = ?
`

	if _, err := tx.ExecContext(ctx, deleteQuery, cloudUUID); err != nil {
		return errors.Trace(err)
	}

	insertQuery := `
INSERT INTO cloud_ca_cert (uuid, cloud_uuid, ca_cert)
VALUES (?, ?, ?)
`
	for _, cert := range certs {

		if _, err := tx.ExecContext(ctx, insertQuery, utils.MustNewUUID().String(), cloudUUID, cert); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func updateRegions(ctx context.Context, tx *sql.Tx, cloudUUID string, regions []cloud.Region) error {
	regionNamesBinds, regionNames := database.SliceToPlaceholderTransform(
		regions, func(r cloud.Region) any {
			return r.Name
		},
	)

	// Delete any regions no longer in the list.
	deleteQuery := fmt.Sprintf(`
DELETE FROM  cloud_region
WHERE        cloud_uuid = ?
AND          name NOT IN (%s)
`, regionNamesBinds)

	args := append([]any{cloudUUID}, regionNames...)
	if _, err := tx.ExecContext(ctx, deleteQuery, args...); err != nil {
		return errors.Trace(err)
	}

	insertQuery := `
INSERT INTO cloud_region (uuid, cloud_uuid, name, endpoint, identity_endpoint, storage_endpoint)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(cloud_uuid, name) DO UPDATE SET name=excluded.name,
                                            endpoint=excluded.endpoint,
                                            identity_endpoint=excluded.identity_endpoint,
                                            storage_endpoint=excluded.storage_endpoint
`
	for _, r := range regions {

		if _, err := tx.ExecContext(ctx, insertQuery, utils.MustNewUUID().String(), cloudUUID,
			r.Name, r.Endpoint, r.IdentityEndpoint, r.StorageEndpoint,
		); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func dbCloudFromCloud(ctx context.Context, tx *sql.Tx, cloudUUID string, cloud cloud.Cloud) (*Cloud, error) {
	cld := &Cloud{
		ID:                cloudUUID,
		Name:              cloud.Name,
		Endpoint:          cloud.Endpoint,
		IdentityEndpoint:  cloud.IdentityEndpoint,
		StorageEndpoint:   cloud.StorageEndpoint,
		SkipTLSVerify:     cloud.SkipTLSVerify,
		IsControllerCloud: cloud.IsControllerCloud,
	}

	row := tx.QueryRowContext(ctx, "SELECT id FROM cloud_type WHERE type = ?", cloud.Type)
	err := row.Scan(&cld.TypeID)
	if err == sql.ErrNoRows {
		return nil, errors.NotValidf("cloud type %q", cloud.Type)
	}
	if err != nil {
		return nil, errors.Trace(err)
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
// the given database.
func AllowCloudType(ctx context.Context, db coredatabase.TxnRunner, version int, name string) error {
	return errors.Trace(db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO cloud_type VALUES (?, ?)`, version, name)
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
		if err := row.Scan(&uuid); err == sql.ErrNoRows {
			return fmt.Errorf("cloud %q %w%w", cloudName, errors.NotFound, errors.Hide(err))
		} else if err != nil {
			return fmt.Errorf("fetching cloud %q: %w", cloudName, err)
		}
		return errors.Trace(err)
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
// removed and no further operations will be be
// performed. If no cloud exists for the cloud name then an error satisfying
// [clouderrors.NotFound] is returned.
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
