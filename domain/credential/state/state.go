// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	"github.com/juju/juju/core/changestream"
	corecredential "github.com/juju/juju/core/credential"
	coredatabase "github.com/juju/juju/core/database"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain"
	userstate "github.com/juju/juju/domain/access/state"
	dbcloud "github.com/juju/juju/domain/cloud/state"
	"github.com/juju/juju/domain/credential"
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

func credentialKeyMap(key corecredential.Key) sqlair.M {
	return sqlair.M{
		"credential_name": key.Name,
		"cloud_name":      key.Cloud,
		"owner":           key.Owner,
	}
}

func (st *State) credentialUUID(ctx context.Context, tx *sqlair.TX, key corecredential.Key) (string, error) {
	selectQ := `
SELECT &M.uuid
FROM v_cloud_credential
WHERE name = $M.credential_name
AND owner_name = $M.owner
AND cloud_name = $M.cloud_name
`

	selectStmt, err := st.Prepare(selectQ, sqlair.M{})
	if err != nil {
		return "", errors.Trace(err)
	}
	uuid := sqlair.M{}
	err = tx.Query(ctx, selectStmt, credentialKeyMap(key)).Get(&uuid)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("cloud credential %q %w%w", key, errors.NotFound, errors.Hide(err))
	} else if err != nil {
		return "", fmt.Errorf("fetching cloud credential %q: %w", key, err)
	}
	return uuid["uuid"].(string), nil
}

// UpsertCloudCredential adds or updates a cloud credential with the given name,
// cloud and owner. If the credential exists already, the existing credential's
// Invalid value is returned.
//
// If the owner of the credential can't be found then an error satisfying
// [usererrors.NotFound] will be returned.
func (st *State) UpsertCloudCredential(ctx context.Context, key corecredential.Key, credential credential.CloudCredentialInfo) (*bool, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	q := `
SELECT  uuid AS &M.uuid,
        invalid AS &M.invalid
FROM v_cloud_credential
WHERE cloud_name = $M.cloud_name
AND name = $M.credential_name
AND owner_name = $M.owner
`
	stmt, err := st.Prepare(q, sqlair.M{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	var existingInvalid *bool
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Get the credential UUID - either existing or make a new one.
		// TODO(wallyworld) - implement owner as a FK when users are modelled.

		result := sqlair.M{}
		err = tx.Query(ctx, stmt, credentialKeyMap(key)).Get(result)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Trace(err)
		}
		invalid, ok := result["invalid"].(bool)
		if ok {
			existingInvalid = &invalid
		}
		credentialUUID, ok := result["uuid"].(string)
		if !ok {
			if credential.Invalid || credential.InvalidReason != "" {
				return fmt.Errorf("adding invalid credential %w", errors.NotSupported)
			}
			credentialUUID = uuid.MustNewUUID().String()
		}

		if err := upsertCredential(ctx, tx, credentialUUID, key, credential); err != nil {
			return domain.CoerceError(fmt.Errorf("updating credential: %w", err))
		}

		if err := updateCredentialAttributes(ctx, tx, credentialUUID, credential.Attributes); err != nil {
			return domain.CoerceError(fmt.Errorf("updating credential %q attributes: %w", key.Name, err))
		}

		// TODO(wallyworld) - update model status (suspended etc)

		return nil
	})

	return existingInvalid, errors.Trace(err)
}

// CreateCredential saves the specified credential.
// Exported for use in the related credential bootstrap package.
func CreateCredential(ctx context.Context, tx *sqlair.TX, credentialUUID string, key corecredential.Key, credential credential.CloudCredentialInfo) error {
	if err := upsertCredential(ctx, tx, credentialUUID, key, credential); err != nil {
		return errors.Annotatef(err, "creating credential %s", credentialUUID)
	}
	if err := updateCredentialAttributes(ctx, tx, credentialUUID, credential.Attributes); err != nil {
		return errors.Annotatef(err, "creating credential %s attributes", credentialUUID)
	}
	return nil
}

func upsertCredential(ctx context.Context, tx *sqlair.TX, credentialUUID string, key corecredential.Key, credential credential.CloudCredentialInfo) error {
	dbCredential, err := dbCredentialFromCredential(ctx, tx, credentialUUID, key, credential)
	if err != nil {
		return errors.Trace(err)
	}

	insertQuery := `
INSERT INTO cloud_credential (uuid, name, cloud_uuid, auth_type_id, owner_uuid, revoked, invalid, invalid_reason)
VALUES (
    $Credential.uuid,
    $Credential.name,
    $Credential.cloud_uuid,
    $Credential.auth_type_id,
    $Credential.owner_uuid,
    $Credential.revoked,
    $Credential.invalid,
    $Credential.invalid_reason
)
ON CONFLICT(uuid) DO UPDATE SET name=excluded.name,
                                cloud_uuid=excluded.cloud_uuid,
                                auth_type_id=excluded.auth_type_id,
                                owner_uuid=excluded.owner_uuid,
                                revoked=excluded.revoked,
                                invalid=excluded.invalid,
                                invalid_reason=excluded.invalid_reason
`

	insertStmt, err := sqlair.Prepare(insertQuery, Credential{})
	if err != nil {
		return errors.Trace(err)
	}

	err = tx.Query(ctx, insertStmt, dbCredential).Run()
	if database.IsErrConstraintCheck(err) {
		return fmt.Errorf("credential name cannot be empty%w%w", errors.Hide(errors.NotValid), errors.Hide(err))
	} else if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func updateCredentialAttributes(ctx context.Context, tx *sqlair.TX, credentialUUID string, attr credentialAttrs) error {
	// Delete any keys no longer in the attributes map.
	// TODO(wallyworld) - sqlair does not support IN operations with a list of values
	deleteQuery := fmt.Sprintf(`
DELETE FROM  cloud_credential_attributes
WHERE        cloud_credential_uuid = $M.uuid
-- AND          key NOT IN (?)
`)

	deleteStmt, err := sqlair.Prepare(deleteQuery, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}
	if err := tx.Query(ctx, deleteStmt, sqlair.M{"uuid": credentialUUID}).Run(); err != nil {
		return errors.Trace(err)
	}

	insertQuery := `
INSERT INTO cloud_credential_attributes
VALUES (
    $credentialAttribute.cloud_credential_uuid,
    $credentialAttribute.key,
    $credentialAttribute.value
)
ON CONFLICT(cloud_credential_uuid, key) DO UPDATE SET key=excluded.key,
                                                      value=excluded.value
`
	insertStmt, err := sqlair.Prepare(insertQuery, credentialAttribute{})
	if err != nil {
		return errors.Trace(err)
	}

	for key, value := range attr {
		if err := tx.Query(ctx, insertStmt, credentialAttribute{
			CredentialUUID: credentialUUID,
			Key:            key,
			Value:          value,
		}).Run(); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// dbCredentialFromCredential is responsible for populating a database
// representation of a cloud credential from a credential id and info structures.
//
// If no user is found for the credential owner then an error satisfying
// [usererrors.NotFound] will be returned.
func dbCredentialFromCredential(ctx context.Context, tx *sqlair.TX, credentialUUID string, key corecredential.Key, credential credential.CloudCredentialInfo) (*Credential, error) {
	cred := &Credential{
		ID:            credentialUUID,
		Name:          key.Name,
		AuthTypeID:    -1,
		Revoked:       credential.Revoked,
		Invalid:       credential.Invalid,
		InvalidReason: credential.InvalidReason,
	}

	userUUID, err := userstate.GetUserUUIDByName(ctx, tx, key.Owner)
	if err != nil {
		return nil, fmt.Errorf("getting user uuid for credential owner %q: %w", key.Owner, err)
	}
	cred.OwnerUUID = userUUID.String()

	q := "SELECT uuid AS &Credential.cloud_uuid FROM cloud WHERE name = $Cloud.name"
	stmt, err := sqlair.Prepare(q, Credential{}, dbcloud.Cloud{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	err = tx.Query(ctx, stmt, dbcloud.Cloud{Name: key.Cloud}).Get(cred)
	if err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil, fmt.Errorf("cloud %q for credential %w", key.Cloud, errors.NotFound)
		}
		return nil, errors.Trace(err)
	}

	validAuthTypes, err := validAuthTypesForCloud(ctx, tx, key.Cloud)
	if err != nil {
		return nil, errors.Annotate(err, "loading cloud auth types")
	}
	if errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Annotate(err, "no valid cloud auth types")
	}
	var validAuthTypeNames []string
	for _, at := range validAuthTypes {
		if at.Type == credential.AuthType {
			cred.AuthTypeID = at.ID
		}
		validAuthTypeNames = append(validAuthTypeNames, at.Type)
	}
	if cred.AuthTypeID == -1 {
		return nil, fmt.Errorf(
			"validating credential %q owned by %q for cloud %q: supported auth-types %q, %q %w",
			key.Name, key.Owner, key.Cloud, validAuthTypeNames, credential.AuthType, errors.NotSupported)
	}
	return cred, nil
}

func validAuthTypesForCloud(ctx context.Context, tx *sqlair.TX, cloudName string) ([]dbcloud.AuthType, error) {
	authTypeQuery := `
SELECT &AuthType.*
FROM   auth_type
JOIN   cloud_auth_type ON auth_type.id = cloud_auth_type.auth_type_id
JOIN   cloud ON cloud_auth_type.cloud_uuid = cloud.uuid
WHERE  cloud.name = $Cloud.name
`
	stmt, err := sqlair.Prepare(authTypeQuery, dbcloud.AuthType{}, dbcloud.Cloud{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	var result dbcloud.AuthTypes
	err = tx.Query(ctx, stmt, dbcloud.Cloud{Name: cloudName}).GetAll(&result)
	return result, errors.Trace(err)
}

// InvalidateCloudCredential marks a cloud credential with the given name, cloud and owner. as invalid.
func (st *State) InvalidateCloudCredential(ctx context.Context, key corecredential.Key, reason string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	q := `
UPDATE cloud_credential
SET    invalid = true, invalid_reason = $M.invalid_reason
FROM cloud
WHERE  cloud_credential.name = $M.credential_name
AND    cloud_credential.owner_uuid = (
    SELECT uuid
    FROM user
    WHERE user.name = $M.owner
	AND user.removed = false
)
AND    cloud_credential.cloud_uuid = (
    SELECT uuid FROM cloud
    WHERE name = $M.cloud_name
)`
	stmt, err := st.Prepare(q, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var outcome sqlair.Outcome
		terms := credentialKeyMap(key)
		terms["invalid_reason"] = reason
		err = tx.Query(ctx, stmt, terms).Get(&outcome)
		if err != nil {
			return errors.Trace(err)
		}
		n, err := outcome.Result().RowsAffected()
		if err != nil {
			return errors.Trace(err)
		}
		if n < 1 {
			return fmt.Errorf("credential %q for cloud %q owned by %q %w", key.Name, key.Cloud, key.Owner, errors.NotFound)
		}
		return nil
	})
	return errors.Trace(err)
}

// CloudCredentialsForOwner returns the owner's cloud credentials for a given cloud,
// keyed by credential name.
func (st *State) CloudCredentialsForOwner(ctx context.Context, owner, cloudName string) (map[string]credential.CloudCredentialResult, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var creds []credential.CloudCredentialResult
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		creds, err = st.loadCloudCredentials(ctx, tx, "", cloudName, owner)
		return errors.Trace(err)
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := make(map[string]credential.CloudCredentialResult)
	for _, cred := range creds {
		result[fmt.Sprintf("%s/%s/%s", cloudName, owner, cred.Label)] = cred
	}
	return result, nil
}

// CloudCredential returns the cloud credential for the given details.
func (st *State) CloudCredential(ctx context.Context, key corecredential.Key) (credential.CloudCredentialResult, error) {
	db, err := st.DB()
	if err != nil {
		return credential.CloudCredentialResult{}, errors.Trace(err)
	}

	var creds []credential.CloudCredentialResult
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		creds, err = st.loadCloudCredentials(ctx, tx, key.Name, key.Cloud, key.Owner)
		return errors.Trace(err)
	})
	if len(creds) == 0 {
		return credential.CloudCredentialResult{}, fmt.Errorf("credential %q for cloud %q owned by %q %w", key.Name, key.Cloud, key.Owner, errors.NotFound)
	}
	if err != nil {
		return credential.CloudCredentialResult{}, errors.Trace(err)
	}
	if len(creds) > 1 {
		return credential.CloudCredentialResult{}, errors.Errorf("expected 1 credential, got %d", len(creds))
	}
	return creds[0], errors.Trace(err)
}

type credentialAttrs map[string]string

func (st *State) loadCloudCredentials(ctx context.Context, tx *sqlair.TX, name, cloudName, owner string) ([]credential.CloudCredentialResult, error) {
	credQuery := `
SELECT (cc.uuid, cc.name,
       cc.revoked, cc.invalid, 
       cc.invalid_reason, 
       cc.owner_uuid) AS (&Credential.*),
       auth_type.type AS &AuthType.*,
       cloud.name AS &Cloud.*,
       (cc_attr.key, cc_attr.value) AS (&credentialAttribute.*)
FROM   cloud_credential cc
       JOIN auth_type ON cc.auth_type_id = auth_type.id
       JOIN cloud ON cc.cloud_uuid = cloud.uuid
	   JOIN user on cc.owner_uuid = user.uuid
       LEFT JOIN cloud_credential_attributes cc_attr ON cc_attr.cloud_credential_uuid = cc.uuid
`

	condition, args := database.SqlairClauseAnd(map[string]any{
		"cc.name":      name,
		"cloud.name":   cloudName,
		"user.name":    owner,
		"user.removed": false,
	})
	types := []any{
		Credential{},
		dbcloud.AuthType{},
		dbcloud.Cloud{},
		credentialAttribute{},
	}
	var queryArgs []any
	if len(args) > 0 {
		credQuery = credQuery + "WHERE " + condition
		types = append(types, sqlair.M{})
		queryArgs = []any{args}
	}

	credStmt, err := st.Prepare(credQuery, types...)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var (
		dbRows      Credentials
		dbAuthTypes []dbcloud.AuthType
		dbclouds    []dbcloud.Cloud
		keyValues   []credentialAttribute
	)
	err = tx.Query(ctx, credStmt, queryArgs...).GetAll(&dbRows, &dbAuthTypes, &dbclouds, &keyValues)
	if err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil, nil
		}
		return nil, errors.Annotate(err, "loading cloud credentials")
	}
	return dbRows.toCloudCredentials(dbAuthTypes, dbclouds, keyValues)
}

// AllCloudCredentialsForOwner returns all cloud credentials stored on the controller
// for a given owner.
func (st *State) AllCloudCredentialsForOwner(ctx context.Context, owner string) (map[corecredential.Key]credential.CloudCredentialResult, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	result := make(map[corecredential.Key]credential.CloudCredentialResult)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		infos, err := st.loadCloudCredentials(ctx, tx, "", "", owner)
		for _, info := range infos {
			result[corecredential.Key{
				Cloud: info.CloudName,
				Owner: owner,
				Name:  info.Label,
			}] = info
		}
		return errors.Trace(err)
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("cloud credentials for %q %w", owner, errors.NotFound)
	}
	return result, errors.Trace(err)
}

// RemoveCloudCredential removes a cloud credential with the given name, cloud and owner..
func (st *State) RemoveCloudCredential(ctx context.Context, key corecredential.Key) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	credAttrDeleteQ := `
DELETE FROM cloud_credential_attributes
WHERE  cloud_credential_attributes.cloud_credential_uuid = $M.uuid
`

	credDeleteQ := `
DELETE FROM cloud_credential
WHERE  cloud_credential.uuid = $M.uuid
`

	credAttrDeleteStmt, err := st.Prepare(credAttrDeleteQ, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}
	credDeleteStmt, err := st.Prepare(credDeleteQ, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		uuid, err := st.credentialUUID(ctx, tx, key)
		if err != nil {
			return errors.Trace(err)
		}
		uuidMap := sqlair.M{"uuid": uuid}
		if err := tx.Query(ctx, credAttrDeleteStmt, uuidMap).Run(); err != nil {
			return errors.Annotate(err, "deleting credential attributes")
		}
		err = tx.Query(ctx, credDeleteStmt, uuidMap).Run()
		return errors.Annotate(err, "deleting credential")
	})
}

// WatchCredential returns a new NotifyWatcher watching for changes to the specified credential.
func (st *State) WatchCredential(
	ctx context.Context,
	getWatcher func(string, string, changestream.ChangeType) (watcher.NotifyWatcher, error),
	key corecredential.Key,
) (watcher.NotifyWatcher, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var uuid string
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		uuid, err = st.credentialUUID(ctx, tx, key)
		return errors.Trace(err)
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	result, err := getWatcher("cloud_credential", uuid, changestream.All)
	return result, errors.Annotatef(err, "watching credential")
}

// ModelsUsingCloudCredential returns a map of uuid->name for models which use the credential.
func (st *State) ModelsUsingCloudCredential(ctx context.Context, key corecredential.Key) (map[coremodel.UUID]string, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	query := `
SELECT mm.model_uuid AS &M.model_uuid, mm.name AS &M.name
FROM   model_metadata mm
JOIN cloud_credential cc ON cc.uuid = mm.cloud_credential_uuid
JOIN cloud ON cloud.uuid = cc.cloud_uuid
JOIN user ON cc.owner_uuid = user.uuid
`

	types := []any{
		sqlair.M{},
	}
	condition, args := database.SqlairClauseAnd(map[string]any{
		"cc.name":      key.Name,
		"cloud.name":   key.Cloud,
		"user.name":    key.Owner,
		"user.removed": false,
	})
	query = query + "WHERE " + condition

	stmt, err := st.Prepare(query, types...)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var info []sqlair.M
	result := make(map[coremodel.UUID]string)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt, args).GetAll(&info)
	})
	if err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return result, nil
		}
		return nil, errors.Trace(err)
	}
	for _, m := range info {
		name, _ := m["name"].(string)
		uuid, _ := m["model_uuid"].(string)
		result[coremodel.UUID(uuid)] = name
	}
	return result, nil
}
