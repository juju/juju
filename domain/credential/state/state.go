// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"
	"github.com/juju/utils/v3"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/changestream"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain"
	dbcloud "github.com/juju/juju/domain/cloud/state"
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

func credentialKeyMap(name, cloudName, owner string) sqlair.M {
	return sqlair.M{
		"credential_name": name,
		"cloud_name":      cloudName,
		"owner":           owner,
	}
}

func (st *State) credentialUUID(ctx context.Context, tx *sqlair.TX, name, cloudName, owner string) (string, error) {
	selectQ := `
SELECT &M.uuid FROM cloud_credential
WHERE  cloud_credential.name = $M.credential_name
AND    cloud_credential.owner_uuid = $M.owner
AND    cloud_credential.cloud_uuid = (
    SELECT uuid FROM cloud
    WHERE name = $M.cloud_name
)`

	selectStmt, err := sqlair.Prepare(selectQ, sqlair.M{})
	if err != nil {
		return "", errors.Trace(err)
	}
	uuid := sqlair.M{}
	err = tx.Query(ctx, selectStmt, credentialKeyMap(name, cloudName, owner)).Get(&uuid)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("cloud credential %s/%s/%s %w%w", cloudName, owner, name, errors.NotFound, errors.Hide(err))
	} else if err != nil {
		return "", fmt.Errorf("fetching cloud credential %s/%s/%s: %w", cloudName, owner, name, err)
	}
	return uuid["uuid"].(string), nil
}

// UpsertCloudCredential adds or updates a cloud credential with the given name, cloud and owner.
func (st *State) UpsertCloudCredential(ctx context.Context, name, cloudName, owner string, credential cloud.Credential) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	q := `
SELECT  cloud_credential.uuid AS &M.uuid
FROM    cloud_credential
        JOIN cloud ON cloud_credential.cloud_uuid = cloud.uuid
WHERE   cloud.name = $M.cloud_name AND cloud_credential.name = $M.credential_name AND cloud_credential.owner_uuid = $M.owner
`
	stmt, err := sqlair.Prepare(q, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Get the credential UUID - either existing or make a new one.
		// TODO(wallyworld) - implement owner as a FK when users are modelled.

		result := sqlair.M{}
		err = tx.Query(ctx, stmt, credentialKeyMap(name, cloudName, owner)).Get(result)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Trace(err)
		}
		credentialUUID, ok := result["uuid"].(string)
		if !ok {
			if credential.Invalid || credential.InvalidReason != "" {
				return fmt.Errorf("adding invalid credential %w", errors.NotSupported)
			}
			credentialUUID = utils.MustNewUUID().String()
		}

		if err := upsertCredential(ctx, tx, credentialUUID, name, cloudName, owner, credential); err != nil {
			return errors.Annotate(err, "updating credential")
		}

		if err := updateCredentialAttributes(ctx, tx, credentialUUID, credential.Attributes()); err != nil {
			return errors.Annotate(err, "updating credential attributes")
		}

		// TODO(wallyworld) - update model status (suspended etc)

		return nil
	})

	return errors.Trace(err)
}

// CreateCredential saves the specified credential.
// Exported for use in the related credential bootstrap package.
func CreateCredential(ctx context.Context, tx *sqlair.TX, credentialUUID string, name, cloud, owner string, credential cloud.Credential) error {
	if err := upsertCredential(ctx, tx, credentialUUID, name, cloud, owner, credential); err != nil {
		return errors.Annotatef(err, "creating credential %s", credentialUUID)
	}
	if err := updateCredentialAttributes(ctx, tx, credentialUUID, credential.Attributes()); err != nil {
		return errors.Annotatef(err, "creating credential %s attributes", credentialUUID)
	}
	return nil
}

func upsertCredential(ctx context.Context, tx *sqlair.TX, credentialUUID string, name, cloud, owner string, credential cloud.Credential) error {
	dbCredential, err := dbCredentialFromCredential(ctx, tx, credentialUUID, name, cloud, owner, credential)
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

func dbCredentialFromCredential(ctx context.Context, tx *sqlair.TX, credentialUUID string, name, cloudName, owner string, credential cloud.Credential) (*Credential, error) {
	cred := &Credential{
		ID:         credentialUUID,
		Name:       name,
		AuthTypeID: -1,
		// TODO(wallyworld) - implement owner as a FK when users are modelled.
		OwnerUUID:     owner,
		Revoked:       credential.Revoked,
		Invalid:       credential.Invalid,
		InvalidReason: credential.InvalidReason,
	}

	q := "SELECT uuid AS &Credential.cloud_uuid FROM cloud WHERE name = $Cloud.name"
	stmt, err := sqlair.Prepare(q, Credential{}, dbcloud.Cloud{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	err = tx.Query(ctx, stmt, dbcloud.Cloud{Name: cloudName}).Get(cred)
	if err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil, fmt.Errorf("cloud %q for credential %w", cloudName, errors.NotFound)
		}
		return nil, errors.Trace(err)
	}

	validAuthTypes, err := validAuthTypesForCloud(ctx, tx, cloudName)
	if err != nil {
		return nil, errors.Annotate(err, "loading cloud auth types")
	}
	var validAuthTypeNames []string
	for _, at := range validAuthTypes {
		if at.Type == string(credential.AuthType()) {
			cred.AuthTypeID = at.ID
		}
		validAuthTypeNames = append(validAuthTypeNames, at.Type)
	}
	if cred.AuthTypeID == -1 {
		return nil, fmt.Errorf("validating credential %q owned by %q for cloud %q: supported auth-types %q, %q %w", name, owner, cloudName, validAuthTypeNames, credential.AuthType(), errors.NotSupported)
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
func (st *State) InvalidateCloudCredential(ctx context.Context, name, cloudName, owner, reason string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	q := `
UPDATE cloud_credential
SET    invalid = true, invalid_reason = $M.invalid_reason
WHERE  cloud_credential.name = $M.credential_name
AND    cloud_credential.owner_uuid = $M.owner
AND    cloud_credential.cloud_uuid = (
    SELECT uuid FROM cloud
    WHERE name = $M.cloud_name
)`
	stmt, err := sqlair.Prepare(q, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var outcome sqlair.Outcome
		terms := credentialKeyMap(name, cloudName, owner)
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
			return fmt.Errorf("credential %q for cloud %q owned by %q %w", name, cloudName, owner, errors.NotFound)
		}
		return nil
	})
	return errors.Trace(err)
}

// CloudCredentials returns the user's cloud credentials for a given cloud,
// keyed by credential name.
func (st *State) CloudCredentials(ctx context.Context, owner, cloudName string) (map[string]cloud.Credential, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var creds []CloudCredential
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		creds, err = st.loadCloudCredentials(ctx, tx, "", cloudName, owner)
		return errors.Trace(err)
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := make(map[string]cloud.Credential)
	for _, cred := range creds {
		result[fmt.Sprintf("%s/%s/%s", cloudName, owner, cred.Credential.Label)] = cred.Credential
	}
	return result, nil
}

// CloudCredential returns the cloud credential for the given details.
func (st *State) CloudCredential(ctx context.Context, name, cloudName, owner string) (cloud.Credential, error) {
	db, err := st.DB()
	if err != nil {
		return cloud.Credential{}, errors.Trace(err)
	}

	var creds []CloudCredential
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		creds, err = st.loadCloudCredentials(ctx, tx, name, cloudName, owner)
		return errors.Trace(err)
	})
	if err != nil {
		return cloud.Credential{}, errors.Trace(err)
	}
	if len(creds) == 0 {
		return cloud.Credential{}, fmt.Errorf("credential %q for cloud %q owned by %q %w", name, cloudName, owner, errors.NotFound)
	}
	if len(creds) > 1 {
		return cloud.Credential{}, errors.Errorf("expected 1 credential, got %d", len(creds))
	}
	return creds[0].Credential, errors.Trace(err)
}

type credentialAttrs map[string]string

func (st *State) loadCloudCredentials(ctx context.Context, tx *sqlair.TX, name, cloudName, owner string) ([]CloudCredential, error) {
	credQuery := `
SELECT (cc.uuid, cc.name,
       cc.revoked, cc.invalid, 
       cc.invalid_reason, 
       cc.owner_uuid) AS &Credential.*,
       auth_type.type AS &AuthType.*,
       cloud.name AS &Cloud.*,
       (cc_attr.key, cc_attr.value) AS &credentialAttribute.*
FROM   cloud_credential cc
       JOIN auth_type ON cc.auth_type_id = auth_type.id
       JOIN cloud ON cc.cloud_uuid = cloud.uuid
       LEFT JOIN cloud_credential_attributes cc_attr ON cc_attr.cloud_credential_uuid = cc.uuid
`

	types := []any{
		Credential{},
		dbcloud.AuthType{},
		dbcloud.Cloud{},
		credentialAttribute{},
	}
	condition, args := database.SqlairClauseAnd(map[string]any{
		"cc.name":       name,
		"cloud.name":    cloudName,
		"cc.owner_uuid": owner,
	})
	if len(args) > 0 {
		credQuery = credQuery + "WHERE " + condition
		types = append(types, sqlair.M{})
	}

	credStmt, err := sqlair.Prepare(credQuery, types...)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var (
		dbRows      Credentials
		dbAuthTypes []dbcloud.AuthType
		dbclouds    []dbcloud.Cloud
		keyValues   []credentialAttribute
	)
	err = tx.Query(ctx, credStmt, args).GetAll(&dbRows, &dbAuthTypes, &dbclouds, &keyValues)
	if err != nil {
		return nil, errors.Annotate(err, "loading cloud credentials")
	}
	return dbRows.toCloudCredentials(dbAuthTypes, dbclouds, keyValues)
}

// CloudCredential represents a credential and the cloud it belongs to.
type CloudCredential struct {
	Credential cloud.Credential
	CloudName  string
}

// AllCloudCredentials returns all cloud credentials stored on the controller
// for a given user.
func (st *State) AllCloudCredentials(ctx context.Context, owner string) ([]CloudCredential, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var result []CloudCredential
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		result, err = st.loadCloudCredentials(ctx, tx, "", "", owner)
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
func (st *State) RemoveCloudCredential(ctx context.Context, name, cloudName, owner string) error {
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

	credAttrDeleteStmt, err := sqlair.Prepare(credAttrDeleteQ, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}
	credDeleteStmt, err := sqlair.Prepare(credDeleteQ, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		uuid, err := st.credentialUUID(ctx, tx, name, cloudName, owner)
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
	name, cloudName, owner string,
) (watcher.NotifyWatcher, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var uuid string
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		uuid, err = st.credentialUUID(ctx, tx, name, cloudName, owner)
		return errors.Trace(err)
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	result, err := getWatcher("cloud_credential", uuid, changestream.All)
	return result, errors.Annotatef(err, "watching credential")
}
