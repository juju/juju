// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/changestream"
	corecloud "github.com/juju/juju/core/cloud"
	coredatabase "github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain"
	accesserrors "github.com/juju/juju/domain/access/errors"
	clouderrors "github.com/juju/juju/domain/cloud/errors"
	"github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
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

// ListClouds lists the clouds with the specified filter, if any.
func (st *State) ListClouds(ctx context.Context) ([]cloud.Cloud, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	var result []cloud.Cloud
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		result, err = LoadClouds(ctx, st, tx, "")
		return errors.Capture(err)
	})
	return result, errors.Capture(err)
}

// Cloud returns the cloud with the specified name.
func (st *State) Cloud(ctx context.Context, name string) (*cloud.Cloud, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	var result *cloud.Cloud
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		clouds, err := LoadClouds(ctx, st, tx, name)
		if err != nil {
			return errors.Capture(err)
		}
		if len(clouds) == 0 {
			return errors.Errorf("%w %q", clouderrors.NotFound, name)
		}
		result = &clouds[0]
		return nil
	})
	return result, errors.Capture(err)
}

// GetCloudForUUID returns the cloud associated with the provided uuid. If no
// cloud is found for the given uuid then a [clouderrors.NotFound] error is
// returned.
func (st *State) GetCloudForUUID(ctx context.Context, id corecloud.UUID) (cloud.Cloud, error) {
	db, err := st.DB()
	if err != nil {
		return cloud.Cloud{}, errors.Capture(err)
	}

	var rval cloud.Cloud
	return rval, db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		rval, err = GetCloudForUUID(ctx, st, tx, id)
		return err
	})
}

// GetCloudForUUID returns the cloud associated with the provided uuid. If no
// cloud is found for the given id then a [clouderrors.NotFound] error is
// returned.
func GetCloudForUUID(
	ctx context.Context,
	st domain.Preparer,
	tx *sqlair.TX,
	uuid corecloud.UUID,
) (cloud.Cloud, error) {
	cloudID := cloudID{
		UUID: uuid.String(),
	}

	q := `
	SELECT (*) AS (&cloudWithAuthType.*)
    FROM v_cloud_auth
	WHERE uuid = $cloudID.uuid
`

	stmt, err := st.Prepare(q, cloudID, cloudWithAuthType{})
	if err != nil {
		return cloud.Cloud{}, errors.Capture(err)
	}

	var records []cloudWithAuthType
	err = tx.Query(ctx, stmt, cloudID).GetAll(&records)
	if errors.Is(err, sqlair.ErrNoRows) {
		return cloud.Cloud{}, errors.Errorf("%w for uuid %q", clouderrors.NotFound, uuid)
	} else if err != nil {
		return cloud.Cloud{}, errors.Errorf("getting cloud %q: %w", uuid, err)
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

	caCerts, err := loadCACerts(ctx, tx, []string{uuid.String()})
	if err != nil {
		return cloud.Cloud{}, errors.Errorf("loading cloud %q ca certificates: %w", uuid, err)
	}
	cld.CACertificates = caCerts[uuid.String()]

	regions, err := loadRegions(ctx, tx, []string{uuid.String()})
	if err != nil {
		return cloud.Cloud{}, errors.Errorf("loading cloud %q regions: %w", uuid, err)
	}
	cld.Regions = regions[uuid.String()]

	return cld, nil
}

// LoadClouds loads the cloud information from the database for the provided name.
func LoadClouds(ctx context.Context, st domain.Preparer, tx *sqlair.TX, name string) ([]cloud.Cloud, error) {
	q := `
	SELECT (uuid, name, cloud_type, cloud_type_id, endpoint,
            identity_endpoint, storage_endpoint, skip_tls_verify,
            is_controller_cloud) AS (&dbCloud.*),
            auth_type AS &M.cloud_auth_type
    FROM v_cloud_auth
`

	var args []any
	if name != "" {
		q += "WHERE name = $M.cloud_name"
		args = append(args, sqlair.M{
			"cloud_name": name,
		})
	}

	loadCloudStmt, err := st.Prepare(q, sqlair.M{}, dbCloud{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	iter := tx.Query(ctx, loadCloudStmt, args...).Iter()
	defer func() { _ = iter.Close() }()

	clouds := make(map[string]*cloud.Cloud)
	m := sqlair.M{}
	for iter.Next() {
		var dbCloud dbCloud
		if err := iter.Get(&dbCloud, m); err != nil {
			return nil, errors.Capture(err)
		}
		cld, ok := clouds[dbCloud.UUID]
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
			clouds[dbCloud.UUID] = cld
		}
		// "cloud_auth_type" will be in the map since iter.Get succeeded but may be set to nil.
		if cloudAuthType, ok := m["cloud_auth_type"]; !ok {
			return nil, errors.Errorf("error getting cloud type from database")
		} else if cloudAuthType != nil {
			cld.AuthTypes = append(cld.AuthTypes, cloud.AuthType(cloudAuthType.(string)))
		}
	}
	if err := iter.Close(); err != nil {
		return nil, errors.Capture(err)
	}

	var uuids []string
	for uuid := range clouds {
		uuids = append(uuids, uuid)
	}

	// Add in the ca certs and regions.
	caCerts, err := loadCACerts(ctx, tx, uuids)
	if err != nil {
		return nil, errors.Capture(err)
	}
	for uuid, certs := range caCerts {
		clouds[uuid].CACertificates = certs
	}

	cloudRegions, err := loadRegions(ctx, tx, uuids)
	if err != nil {
		return nil, errors.Capture(err)
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
SELECT &cloudCACert.*
FROM   cloud_ca_cert
WHERE  cloud_uuid IN ($uuids[:])
`, uuids{}, cloudCACert{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var dbCloudCACerts []cloudCACert
	err = tx.Query(ctx, loadCACertStmt, uuids(cloudUUIDs)).GetAll(&dbCloudCACerts)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Capture(err)
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
SELECT &cloudRegion.*
FROM   cloud_region
WHERE  cloud_uuid IN ($uuids[:])
`[1:], uuids{}, cloudRegion{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var dbRegions []cloudRegion
	err = tx.Query(ctx, loadRegionsStmt, uuids(cloudUUIDS)).GetAll(&dbRegions)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Capture(err)
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
		return errors.Capture(err)
	}

	selectUUIDStmt, err := st.Prepare(`
SELECT &dbCloud.uuid 
FROM   cloud 
WHERE  name = $dbCloud.name`, dbCloud{})
	if err != nil {
		return errors.Errorf("preparing select cloud uuid statement: %w", err)
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Get the cloud UUID
		dbCloud := dbCloud{Name: cloud.Name}
		err := tx.Query(ctx, selectUUIDStmt, dbCloud).Get(&dbCloud)
		if err != nil && errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("%q %w", cloud.Name, clouderrors.NotFound)
		} else if err != nil {
			return errors.Capture(err)
		}
		cloudUUID := dbCloud.UUID

		if err := updateCloud(ctx, tx, cloudUUID, cloud); err != nil {
			return errors.Errorf("updating cloud regions: %w", err)
		}
		return nil
	})

	return errors.Capture(err)
}

// CreateCloud creates a cloud and provides admin permissions to the
// provided ownerName.
// This is the exported method for use with the cloud state.
func (st *State) CreateCloud(ctx context.Context, ownerName user.Name, cloudUUID string, cloud cloud.Cloud) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return CreateCloud(ctx, tx, ownerName, cloudUUID, cloud)
	})
	return errors.Capture(err)
}

// CreateCloud saves the specified cloud and creates Admin permission on the
// cloud for the provided user.
// Exported for use in the related cloud bootstrap package.
// Should never be directly called outside of the cloud bootstrap package.
func CreateCloud(ctx context.Context, tx *sqlair.TX, ownerName user.Name, cloudUUID string, cloud cloud.Cloud) error {
	if err := updateCloud(ctx, tx, cloudUUID, cloud); err != nil {
		return errors.Errorf("updating cloud %s: %w", cloudUUID, err)
	}
	if err := insertPermission(ctx, tx, ownerName, cloud.Name); err != nil {
		return errors.Errorf("inserting cloud user permission: %w", err)
	}
	return nil
}

func updateCloud(ctx context.Context, tx *sqlair.TX, cloudUUID string, cloud cloud.Cloud) error {
	if err := upsertCloud(ctx, tx, cloudUUID, cloud); err != nil {
		return errors.Errorf("updating cloud %s: %w", cloudUUID, err)
	}
	if err := updateAuthTypes(ctx, tx, cloudUUID, cloud.AuthTypes); err != nil {
		return errors.Errorf("updating cloud %s auth types: %w", cloudUUID, err)
	}
	if err := updateCACerts(ctx, tx, cloudUUID, cloud.CACertificates); err != nil {
		return errors.Errorf("updating cloud %s CA certs: %w", cloudUUID, err)
	}
	if err := updateRegions(ctx, tx, cloudUUID, cloud.Regions); err != nil {
		return errors.Errorf("updating cloud %s regions: %w", cloudUUID, err)
	}
	return nil
}

func upsertCloud(ctx context.Context, tx *sqlair.TX, cloudUUID string, cloud cloud.Cloud) error {
	cloudFromDB, err := dbCloudFromCloud(ctx, tx, cloudUUID, cloud)
	if err != nil {
		return errors.Capture(err)
	}

	insertCloudStmt, err := sqlair.Prepare(`
INSERT INTO cloud (uuid, name, cloud_type_id, endpoint,
                   identity_endpoint, storage_endpoint,
                   skip_tls_verify)
VALUES ($dbCloud.*)
ON CONFLICT(uuid) DO UPDATE SET name=excluded.name,
                                endpoint=excluded.endpoint,
                                identity_endpoint=excluded.identity_endpoint,
                                storage_endpoint=excluded.storage_endpoint,
                                skip_tls_verify=excluded.skip_tls_verify;
`, dbCloud{})
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, insertCloudStmt, cloudFromDB).Run()
	if database.IsErrConstraintCheck(err) {
		return errors.Errorf("%w cloud name cannot be empty", coreerrors.NotValid).Add(err)
	} else if err != nil {
		return errors.Capture(err)
	}
	return nil
}

// loadAuthTypes reads the cloud auth type values and ids
// into a map for easy lookup.
func loadAuthTypes(ctx context.Context, tx *sqlair.TX) (map[string]int, error) {
	var dbAuthTypes = map[string]int{}

	stmt, err := sqlair.Prepare("SELECT &authType.* FROM auth_type", authType{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var authTypes []authType
	err = tx.Query(ctx, stmt).GetAll(&authTypes)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Capture(err)
	}
	for _, authType := range authTypes {
		dbAuthTypes[authType.Type] = authType.ID
	}
	return dbAuthTypes, nil
}

func updateAuthTypes(ctx context.Context, tx *sqlair.TX, cloudUUID string, authTypes cloud.AuthTypes) error {
	dbAuthTypes, err := loadAuthTypes(ctx, tx)
	if err != nil {
		return errors.Capture(err)
	}

	// First validate the passed in auth types.
	var authTypeIds = make(authTypeIds, len(authTypes))
	for i, a := range authTypes {
		id, ok := dbAuthTypes[string(a)]
		if !ok {
			return errors.Errorf("auth type %q %w", a, coreerrors.NotValid)
		}
		authTypeIds[i] = id
	}

	// Delete auth types no longer in the list.
	deleteQuery, err := sqlair.Prepare(`
DELETE FROM  cloud_auth_type
WHERE        cloud_uuid = $M.cloud_uuid
AND          auth_type_id NOT IN ($authTypeIds[:])
`, authTypeIds, sqlair.M{})
	if err != nil {
		return errors.Capture(err)
	}

	if err := tx.Query(ctx, deleteQuery, authTypeIds, sqlair.M{"cloud_uuid": cloudUUID}).Run(); err != nil {
		return errors.Capture(err)
	}

	insertStmt, err := sqlair.Prepare(`
INSERT INTO cloud_auth_type (cloud_uuid, auth_type_id)
VALUES ($cloudAuthType.*)
ON CONFLICT(cloud_uuid, auth_type_id) DO NOTHING;
	`, cloudAuthType{})
	if err != nil {
		return errors.Capture(err)
	}

	for _, a := range authTypeIds {
		cloudAuthType := cloudAuthType{CloudUUID: cloudUUID, AuthTypeID: a}
		if err := tx.Query(ctx, insertStmt, cloudAuthType).Run(); err != nil {
			return errors.Capture(err)
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
		return errors.Capture(err)
	}
	insertQuery, err := sqlair.Prepare(`
INSERT INTO cloud_ca_cert (cloud_uuid, ca_cert)
VALUES ($cloudCACert.*)
`, cloudCACert{})
	if err != nil {
		return errors.Capture(err)
	}

	if err := tx.Query(ctx, deleteQuery, sqlair.M{"cloud_uuid": cloudUUID}).Run(); err != nil {
		return errors.Capture(err)
	}

	for _, cert := range certs {
		cloudCACert := cloudCACert{CloudUUID: cloudUUID, CACert: cert}
		if err := tx.Query(ctx, insertQuery, cloudCACert).Run(); err != nil {
			return errors.Capture(err)
		}
	}
	return nil
}

func updateRegions(ctx context.Context, tx *sqlair.TX, cloudUUID string, regions []cloud.Region) error {
	dbRegionNames := regionNames(transform.Slice(regions, func(r cloud.Region) string { return r.Name }))

	deleteQuery, err := sqlair.Prepare(`
DELETE FROM  cloud_region
WHERE        cloud_uuid = $M.cloud_uuid
AND          name NOT IN ($regionNames[:])
`, regionNames{}, sqlair.M{})
	if err != nil {
		return errors.Capture(err)
	}

	insertQuery, err := sqlair.Prepare(`
INSERT INTO cloud_region (uuid, cloud_uuid, name,
                          endpoint, identity_endpoint,
                          storage_endpoint)
VALUES ($cloudRegion.*)
ON CONFLICT(cloud_uuid, name) DO UPDATE SET name=excluded.name,
                                            endpoint=excluded.endpoint,
                                            identity_endpoint=excluded.identity_endpoint,
                                            storage_endpoint=excluded.storage_endpoint
`, cloudRegion{})
	if err != nil {
		return errors.Capture(err)
	}

	// Delete any regions no longer in the list.
	if err := tx.Query(ctx, deleteQuery, sqlair.M{"cloud_uuid": cloudUUID}, dbRegionNames).Run(); err != nil {
		return errors.Capture(err)
	}

	for _, r := range regions {
		cloudRegion := cloudRegion{UUID: uuid.MustNewUUID().String(),
			CloudUUID: cloudUUID, Name: r.Name, Endpoint: r.Endpoint,
			IdentityEndpoint: r.IdentityEndpoint,
			StorageEndpoint:  r.StorageEndpoint}
		if err := tx.Query(ctx, insertQuery, cloudRegion).Run(); err != nil {
			return errors.Capture(err)
		}
	}
	return nil
}

// insertPermission inserts a permission for the owner of the cloud during
// upsertCloud.
func insertPermission(ctx context.Context, tx *sqlair.TX, ownerName user.Name, cloudName string) error {
	if ownerName.IsZero() {
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
		return errors.Capture(err)
	}

	permUUID, err := uuid.NewUUID()
	if err != nil {
		return errors.Capture(err)
	}
	perm := dbAddUserPermission{
		UUID:       permUUID.String(),
		GrantOn:    cloudName,
		Name:       ownerName.Name(),
		AccessType: string(permission.AdminAccess),
		ObjectType: string(permission.Cloud),
	}

	err = tx.Query(ctx, insertPermissionStmt, perm).Run()
	if err != nil && database.IsErrConstraintUnique(err) {
		return errors.Errorf("for %q on %q, %w", ownerName, cloudName, accesserrors.PermissionAlreadyExists)
	} else if err != nil && (database.IsErrConstraintForeignKey(err) || errors.Is(err, sqlair.ErrNoRows)) {
		return errors.Errorf("%q %w", ownerName, accesserrors.UserNotFound)
	} else if err != nil {
		return errors.Errorf("adding permission %q for %q on %q: %w", string(permission.AdminAccess), ownerName, cloudName, err)
	}

	return nil
}

func dbCloudFromCloud(ctx context.Context, tx *sqlair.TX, cloudUUID string, cloud cloud.Cloud) (*dbCloud, error) {
	cld := &dbCloud{
		UUID:              cloudUUID,
		Name:              cloud.Name,
		Type:              cloud.Type,
		Endpoint:          cloud.Endpoint,
		IdentityEndpoint:  cloud.IdentityEndpoint,
		StorageEndpoint:   cloud.StorageEndpoint,
		SkipTLSVerify:     cloud.SkipTLSVerify,
		IsControllerCloud: cloud.IsControllerCloud,
	}

	selectCloudIDstmt, err := sqlair.Prepare("SELECT id AS &dbCloud.cloud_type_id FROM cloud_type WHERE type = $cloudType.type", dbCloud{}, cloudType{})
	if err != nil {
		return nil, errors.Capture(err)
	}
	cloudType := cloudType{Type: cloud.Type}
	err = tx.Query(ctx, selectCloudIDstmt, cloudType).Get(cld)
	if errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Errorf("cloud type %q %w", cloud.Type, coreerrors.NotValid)
	}
	if err != nil {
		return nil, errors.Capture(err)
	}
	return cld, nil
}

// DeleteCloud removes a cloud credential with the given name.
func (st *State) DeleteCloud(ctx context.Context, name string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	cloudName := dbCloudName{Name: name}
	// TODO(wallyworld) - also check model reference
	cloudDeleteStmt, err := st.Prepare(`
DELETE FROM cloud
WHERE  cloud.name = $dbCloudName.name
AND cloud.uuid NOT IN (
    SELECT cloud_uuid FROM cloud_credential
)
`, cloudName)
	if err != nil {
		return errors.Errorf("preparing delete from cloud statement: %w", err)
	}

	cloudRegionDeleteStmt, err := st.Prepare(`
DELETE FROM cloud_region
    WHERE cloud_uuid IN (
        SELECT uuid FROM cloud WHERE cloud.name = $dbCloudName.name
    )
`, cloudName)
	if err != nil {
		return errors.Errorf("preparing delete from cloud region statement: %w", err)
	}

	cloudCACertDeleteStmt, err := st.Prepare(`
DELETE FROM cloud_ca_cert
    WHERE cloud_uuid IN (
        SELECT uuid FROM cloud WHERE cloud.name = $dbCloudName.name
    )
`, cloudName)
	if err != nil {
		return errors.Errorf("preparing delete from cloud ca cert statement: %w", err)
	}

	cloudAuthTypeDeleteStmt, err := st.Prepare(`
DELETE FROM cloud_auth_type
    WHERE cloud_uuid IN (
        SELECT uuid FROM cloud WHERE cloud.name = $dbCloudName.name
    )
`, cloudName)
	if err != nil {
		return errors.Errorf("preparing delete from cloud auth type statement: %w", err)
	}

	permissionsStmt, err := st.Prepare(`
DELETE FROM permission
WHERE  grant_on = $dbCloudName.name
`, dbCloudName{})
	if err != nil {
		return errors.Errorf("preparing delete cloud from permissions statement: %w", err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, cloudRegionDeleteStmt, cloudName).Run()
		if err != nil {
			return errors.Errorf("deleting cloud regions: %w", err)
		}
		err = tx.Query(ctx, cloudCACertDeleteStmt, cloudName).Run()
		if err != nil {
			return errors.Errorf("deleting cloud ca certs: %w", err)
		}
		err = tx.Query(ctx, cloudAuthTypeDeleteStmt, cloudName).Run()
		if err != nil {
			return errors.Errorf("deleting cloud auth type: %w", err)
		}
		err = tx.Query(ctx, permissionsStmt, cloudName).Run()
		if err != nil {
			return errors.Errorf("deleting permissions on cloud: %w", err)
		}
		var outcome sqlair.Outcome
		err = tx.Query(ctx, cloudDeleteStmt, cloudName).Get(&outcome)
		if err != nil {
			return errors.Errorf("deleting cloud: %w", err)
		}
		num, err := outcome.Result().RowsAffected()
		if err != nil {
			return errors.Capture(err)
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
	dbCloudType := cloudType{
		ID:   version,
		Type: name,
	}
	stmt, err := sqlair.Prepare(`
INSERT INTO cloud_type (*) 
VALUES      ($cloudType.*)`, dbCloudType)
	if err != nil {
		return errors.Errorf("preparing insert cloud type statement: %w", err)
	}
	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, dbCloudType).Run()
		if database.IsErrConstraintUnique(err) {
			return nil
		}
		return err
	}))

}

// WatchCloud returns a new NotifyWatcher watching for changes to the specified cloud.
func (st *State) WatchCloud(
	ctx context.Context,
	getWatcher func(
		summary string,
		filter eventsource.FilterOption,
		filterOpts ...eventsource.FilterOption,
	) (watcher.NotifyWatcher, error),
	cloudName string,
) (watcher.NotifyWatcher, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	cloud := cloudID{
		Name: cloudName,
	}
	stmt, err := st.Prepare(`
SELECT &cloudID.uuid 
FROM cloud 
WHERE name = $cloudID.name`, cloud)
	if err != nil {
		return nil, errors.Errorf("preparing select cloud uuid statement: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, cloud).Get(&cloud)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("cloud %q %w", cloudName, coreerrors.NotFound).Add(err)
		} else if err != nil {
			return errors.Errorf("fetching cloud %q: %w", cloudName, err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	result, err := getWatcher(
		fmt.Sprintf("cloud watcher for %q", cloudName),
		eventsource.PredicateFilter("cloud", changestream.All, eventsource.EqualsPredicate(cloud.UUID)),
	)
	if err != nil {
		return result, errors.Errorf("watching cloud: %w", err)
	}
	return result, nil
}
