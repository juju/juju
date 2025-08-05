// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/changestream"
	corecredential "github.com/juju/juju/core/credential"
	coredatabase "github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	coreuser "github.com/juju/juju/core/user"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain"
	userstate "github.com/juju/juju/domain/access/state"
	"github.com/juju/juju/domain/credential"
	credentialerrors "github.com/juju/juju/domain/credential/errors"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
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

// CredentialUUIDForKey finds and returns the uuid for the cloud credential
// identified by key. If no credential is found then an error of
// [credentialerrors.NotFound] is returned.
func (st *State) CredentialUUIDForKey(ctx context.Context, key corecredential.Key) (corecredential.UUID, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Capture(err)
	}

	var rval corecredential.UUID
	return rval, db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		rval, err = st.credentialUUIDForKey(ctx, tx, key)
		return err
	})
}

// credentialUUIDForKey finds and returns the uuid for the cloud credential
// identified by key. If no credential is found then an error of
// [credentialerrors.NotFound] is returned.
func (st *State) credentialUUIDForKey(
	ctx context.Context,
	tx *sqlair.TX,
	key corecredential.Key,
) (corecredential.UUID, error) {
	dbKey := credentialKey{
		CredentialName: key.Name,
		CloudName:      key.Cloud,
		OwnerName:      key.Owner.String(),
	}
	result := credentialUUID{}

	selectStmt, err := st.Prepare(`
SELECT &credentialUUID.uuid
FROM   v_cloud_credential
WHERE  name = $credentialKey.name
AND    owner_name = $credentialKey.owner_name
AND    cloud_name = $credentialKey.cloud_name
`, dbKey, result)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = tx.Query(ctx, selectStmt, dbKey).Get(&result)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errors.Errorf("cloud credential %q %w", key, credentialerrors.NotFound)
	} else if err != nil {
		return "", errors.Errorf("fetching cloud credential %q: %w", key, err)
	}
	return corecredential.UUID(result.UUID), nil
}

// GetModelCredentialStatus returns the credential key and validity status for
// the credential that is in use for the model. This func will only work with
// models that are active. True is returned for credentials that are valid and
// false for credentials that are considered invalid.
// The following errors can be expected:
// - [modelerrors.NotFound] if no model exists for the provided uuid.
// - [credentialerrors.ModelCredentialNotSet] when the model does not have a
// credential set. This is common when the cloud supports empty auth type.
func (st *State) GetModelCredentialStatus(
	ctx context.Context,
	uuid coremodel.UUID,
) (corecredential.Key, bool, error) {
	db, err := st.DB()
	if err != nil {
		return corecredential.Key{}, false, errors.Capture(err)
	}

	modelUUID := modelUUID{UUID: uuid.String()}
	vals := modelCredentialStatus{}
	stmt, err := st.Prepare(`
SELECT &modelCredentialStatus.*
FROM   v_model
WHERE  uuid = $modelUUID.uuid`,
		modelUUID, vals,
	)
	if err != nil {
		return corecredential.Key{}, false, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, modelUUID).Get(&vals)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"model %q does not exist", uuid,
			).Add(modelerrors.NotFound)
		} else if err != nil {
			return errors.Errorf(
				"getting model %q credential status: %w", uuid, err,
			)
		}
		return nil
	})

	if err != nil {
		return corecredential.Key{}, false, errors.Capture(err)
	}

	if !vals.CredentialName.Valid ||
		!vals.CloudName.Valid ||
		!vals.OwnerName.Valid {
		return corecredential.Key{}, false, errors.Errorf(
			"model %q does not have a credential set", uuid,
		).Add(credentialerrors.ModelCredentialNotSet)
	}

	owner, err := coreuser.NewName(vals.OwnerName.String)
	if err != nil {
		return corecredential.Key{}, false, errors.Errorf(
			"parsing owner name %q for model %q cloud credential: %w",
			vals.OwnerName.String, uuid, err,
		)
	}
	credKey := corecredential.Key{
		Name:  vals.CredentialName.String,
		Cloud: vals.CloudName.String,
		Owner: owner,
	}

	return credKey, !vals.Invalid.Bool, nil
}

// UpsertCloudCredential adds or updates a cloud credential with the given name,
// cloud and owner.
//
// If the owner of the credential can't be found then an error satisfying
// [usererrors.NotFound] will be returned.
func (st *State) UpsertCloudCredential(ctx context.Context, key corecredential.Key, credential credential.CloudCredentialInfo) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	dbKey := credentialKey{
		CredentialName: key.Name,
		CloudName:      key.Cloud,
		OwnerName:      key.Owner.String(),
	}
	stmt, err := st.Prepare(`
SELECT uuid AS &credentialUUID.uuid
FROM   v_cloud_credential
WHERE  name = $credentialKey.name
AND    owner_name = $credentialKey.owner_name
AND    cloud_name = $credentialKey.cloud_name
`, dbKey, credentialUUID{})
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Get the credential UUID - either existing or make a new one.
		// TODO(wallyworld) - implement owner as a FK when users are modelled.

		result := credentialUUID{}
		err = tx.Query(ctx, stmt, dbKey).Get(&result)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		} else if err != nil {
			if credential.Invalid || credential.InvalidReason != "" {
				return errors.Errorf("adding invalid credential %w", coreerrors.NotSupported)
			}
			id, err := corecredential.NewUUID()
			if err != nil {
				return errors.Errorf("generating new credential uuid: %w", err)
			}
			result.UUID = id.String()
		}

		if err := upsertCredential(ctx, tx, result.UUID, key, credential); err != nil {
			return errors.Errorf("updating credential: %w", err)
		}

		if err := updateCredentialAttributes(ctx, tx, result.UUID, credential.Attributes); err != nil {
			return errors.Errorf("updating credential %q attributes: %w", key.Name, err)
		}
		return nil
	})

	return errors.Capture(err)
}

// CreateCredential saves the specified credential.
// Exported for use in the related credential bootstrap package.
func CreateCredential(ctx context.Context, tx *sqlair.TX, credentialUUID string, key corecredential.Key, credential credential.CloudCredentialInfo) error {
	if err := upsertCredential(ctx, tx, credentialUUID, key, credential); err != nil {
		return errors.Errorf("creating credential %s: %w", credentialUUID, err)
	}
	if err := updateCredentialAttributes(ctx, tx, credentialUUID, credential.Attributes); err != nil {
		return errors.Errorf("creating credential %s attributes: %w", credentialUUID, err)
	}
	return nil
}

func upsertCredential(ctx context.Context, tx *sqlair.TX, credentialUUID string, key corecredential.Key, credential credential.CloudCredentialInfo) error {
	dbCredential, err := dbCredentialFromCredential(ctx, tx, credentialUUID, key, credential)
	if err != nil {
		return errors.Capture(err)
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
		return errors.Capture(err)
	}

	err = tx.Query(ctx, insertStmt, dbCredential).Run()
	if database.IsErrConstraintCheck(err) {
		return errors.Errorf("credential name cannot be empty").Add(coreerrors.NotValid)
	} else if err != nil {
		return errors.Capture(err)
	}
	return nil
}

func updateCredentialAttributes(ctx context.Context, tx *sqlair.TX, credentialUUID string, attr credentialAttrs) error {
	// Delete any keys no longer in the attributes map.
	// TODO(wallyworld) - sqlair does not support IN operations with a list of values
	deleteQuery := `
DELETE FROM  cloud_credential_attribute
WHERE        cloud_credential_uuid = $M.uuid
`

	deleteStmt, err := sqlair.Prepare(deleteQuery, sqlair.M{})
	if err != nil {
		return errors.Capture(err)
	}
	if err := tx.Query(ctx, deleteStmt, sqlair.M{"uuid": credentialUUID}).Run(); err != nil {
		return errors.Capture(err)
	}

	insertQuery := `
INSERT INTO cloud_credential_attribute
VALUES (
    $CredentialAttribute.cloud_credential_uuid,
    $CredentialAttribute.key,
    $CredentialAttribute.value
)
ON CONFLICT(cloud_credential_uuid, key) DO UPDATE SET key=excluded.key,
                                                      value=excluded.value
`
	insertStmt, err := sqlair.Prepare(insertQuery, CredentialAttribute{})
	if err != nil {
		return errors.Capture(err)
	}

	for key, value := range attr {
		if err := tx.Query(ctx, insertStmt, CredentialAttribute{
			CredentialUUID: credentialUUID,
			Key:            key,
			Value:          value,
		}).Run(); err != nil {
			return errors.Capture(err)
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
		return nil, errors.Errorf("getting user uuid for credential owner %q: %w", key.Owner, err)
	}
	cred.OwnerUUID = userUUID.String()

	q := "SELECT uuid AS &Credential.cloud_uuid FROM cloud WHERE name = $dbCloudName.name"
	stmt, err := sqlair.Prepare(q, Credential{}, dbCloudName{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, dbCloudName{Name: key.Cloud}).Get(cred)
	if err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil, errors.Errorf("cloud %q for credential %w", key.Cloud, coreerrors.NotFound)
		}
		return nil, errors.Capture(err)
	}

	validAuthTypes, err := validAuthTypesForCloud(ctx, tx, key.Cloud)
	if errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Errorf("no valid cloud auth types: %w", err)
	} else if err != nil {
		return nil, errors.Errorf("loading cloud auth types: %w", err)
	}

	var validAuthTypeNames []string
	for _, at := range validAuthTypes {
		if at.Type == credential.AuthType {
			cred.AuthTypeID = at.ID
		}
		validAuthTypeNames = append(validAuthTypeNames, at.Type)
	}
	if cred.AuthTypeID == -1 {
		return nil, errors.Errorf(
			"validating credential %q owned by %q for cloud %q: supported auth-types %q, %q %w",
			key.Name, key.Owner, key.Cloud, validAuthTypeNames, credential.AuthType, coreerrors.NotSupported)

	}
	return cred, nil
}

func validAuthTypesForCloud(ctx context.Context, tx *sqlair.TX, cloudName string) (authTypes, error) {
	authTypeQuery := `
SELECT &authType.*
FROM   auth_type
JOIN   cloud_auth_type ON auth_type.id = cloud_auth_type.auth_type_id
JOIN   cloud ON cloud_auth_type.cloud_uuid = cloud.uuid
WHERE  cloud.name = $dbCloudName.name
`
	cloud := dbCloudName{Name: cloudName}
	stmt, err := sqlair.Prepare(authTypeQuery, authType{}, cloud)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var result authTypes
	err = tx.Query(ctx, stmt, cloud).GetAll(&result)
	return result, errors.Capture(err)
}

// invalidateCloudCredential invalidates the provided cloud credential
// identified by uuid.
// The following errors can be expected:
// - [credentialerrors.NotFound] when no credential is found for the
// given uuid.
func (st *State) invalidateCloudCredential(
	ctx context.Context,
	tx *sqlair.TX,
	uuid corecredential.UUID,
	reason string,
) error {
	credentialUUID := credentialUUID{UUID: uuid.String()}
	invalidReason := credentialInvalidReason{Reason: reason}
	q := `
UPDATE cloud_credential
SET    invalid = true, invalid_reason = $credentialInvalidReason.invalid_reason
WHERE  uuid = $credentialUUID.uuid
`
	stmt, err := st.Prepare(q, credentialUUID, invalidReason)
	if err != nil {
		return errors.Capture(err)
	}

	var outcome sqlair.Outcome
	err = tx.Query(ctx, stmt, credentialUUID, invalidReason).Get(&outcome)
	if err != nil {
		return errors.Capture(err)
	}
	n, err := outcome.Result().RowsAffected()
	if err != nil {
		return errors.Capture(err)
	}
	if n == 0 {
		return errors.Errorf(
			"credential %q does not exist", uuid,
		).Add(credentialerrors.NotFound)
	}
	return nil
}

// InvalidateCloudCredential marks a cloud credential for the provided uuid as
// invalid.
// The following errors can be expected:
// - [credentialerrors.NotFound] when no credential is found for the
// given uuid.
func (st *State) InvalidateCloudCredential(ctx context.Context, uuid corecredential.UUID, reason string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return st.invalidateCloudCredential(ctx, tx, uuid, reason)
	})
}

// InvalidateModelCloudCredential marks the cloud credential that is in use by
// the given model as invalid. This will affect not just the model used to find
// the cloud credential but all models that are using the same cloud credential
// as the model provided.
//
// This function will only work with models that are active.
// The following errors can be expected:
// - [modelerrors.NotFound] if the no model exists for the provided uuid.
// - [credentialerrors.ModelCredentialNotSet] when the model does not have a
// cloud credential set.
func (st *State) InvalidateModelCloudCredential(
	ctx context.Context,
	uuid coremodel.UUID,
	reason string,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	modelUUID := modelUUID{UUID: uuid.String()}
	modelCredentialUUID := modelCredentialUUID{}
	stmt, err := st.Prepare(`
SELECT &modelCredentialUUID.*
FROM   v_model
WHERE  uuid = $modelUUID.uuid`,
		modelUUID, modelCredentialUUID)
	if err != nil {
		return errors.Capture(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, modelUUID).Get(&modelCredentialUUID)
		if errors.Is(err, sql.ErrNoRows) {
			return errors.Errorf(
				"model %q does not exist", uuid,
			).Add(modelerrors.NotFound)
		} else if err != nil {
			return errors.Errorf(
				"getting cloud credential uuid for model %q: %w",
				uuid, err,
			)
		}

		// The model doesn't have a credential set so we return a
		// [credentialerrors.ModelCredentialNotSet] error to let the caller
		// decide what this implies.
		if !modelCredentialUUID.UUID.Valid {
			return errors.Errorf(
				"model %q does not have a cloud credential set", uuid,
			).Add(credentialerrors.ModelCredentialNotSet)
		}
		return st.invalidateCloudCredential(
			ctx,
			tx,
			corecredential.UUID(modelCredentialUUID.UUID.String),
			reason,
		)
	})
}

// CloudCredentialsForOwner returns the owner's cloud credentials for a given
// cloud, keyed by credential name.
func (st *State) CloudCredentialsForOwner(ctx context.Context, owner coreuser.Name, cloudName string) (map[string]credential.CloudCredentialResult, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	var (
		dbRows      Credentials
		dbAuthTypes []authType
		keyValues   []CredentialAttribute
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		credQuery := `
SELECT cc.* AS &Credential.*,
       auth_type.type AS &authType.type,
       (cc_attr.key, cc_attr.value) AS (&CredentialAttribute.*)
FROM   cloud_credential cc
       JOIN auth_type ON cc.auth_type_id = auth_type.id
       JOIN cloud ON cc.cloud_uuid = cloud.uuid
	   JOIN user on cc.owner_uuid = user.uuid
       LEFT JOIN cloud_credential_attribute cc_attr ON cc_attr.cloud_credential_uuid = cc.uuid
WHERE  user.removed = false
AND    user.name = $ownerAndCloudName.owner_name
AND    cloud.name = $ownerAndCloudName.cloud_name
`
		names := ownerAndCloudName{
			OwnerName: owner.Name(),
			CloudName: cloudName,
		}
		credStmt, err := st.Prepare(
			credQuery,
			names,
			Credential{},
			authType{},
			CredentialAttribute{},
		)
		if err != nil {
			return errors.Errorf("preparing select credentials for owner statement: %w", err)
		}

		err = tx.Query(ctx, credStmt, names).GetAll(&dbRows, &dbAuthTypes, &keyValues)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("loading cloud credentials: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	creds, err := dbRows.ToCloudCredentials(cloudName, dbAuthTypes, keyValues)
	if err != nil {
		return nil, errors.Capture(err)
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
		return credential.CloudCredentialResult{}, errors.Capture(err)
	}

	var (
		dbRows      Credentials
		dbAuthTypes []authType
		keyValues   []CredentialAttribute
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		credQuery := `
SELECT cc.* AS &Credential.*,
       auth_type.type AS &authType.type,
       (cc_attr.key, cc_attr.value) AS (&CredentialAttribute.*)
FROM   cloud_credential cc
       JOIN auth_type ON cc.auth_type_id = auth_type.id
       JOIN cloud ON cc.cloud_uuid = cloud.uuid
	   JOIN user on cc.owner_uuid = user.uuid
       LEFT JOIN cloud_credential_attribute cc_attr ON cc_attr.cloud_credential_uuid = cc.uuid
WHERE  user.removed = false
AND    cloud.name = $credentialKey.cloud_name
AND    user.name = $credentialKey.owner_name
AND    cc.name = $credentialKey.name
`
		credKey := credentialKey{
			CredentialName: key.Name,
			CloudName:      key.Cloud,
			OwnerName:      key.Owner.Name(),
		}
		credStmt, err := st.Prepare(
			credQuery,
			credKey,
			Credential{},
			authType{},
			CredentialAttribute{},
		)
		if err != nil {
			return errors.Capture(err)
		}

		err = tx.Query(ctx, credStmt, credKey).GetAll(&dbRows, &dbAuthTypes, &keyValues)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("loading cloud credentials: %w", err)
		}
		return nil
	})
	if err != nil {
		return credential.CloudCredentialResult{}, errors.Capture(err)
	}
	if len(dbRows) == 0 {
		return credential.CloudCredentialResult{}, errors.Errorf(
			"%w: credential %q for cloud %q owned by %q",
			credentialerrors.NotFound, key.Name, key.Cloud, key.Owner)

	}
	creds, err := dbRows.ToCloudCredentials(key.Cloud, dbAuthTypes, keyValues)
	if err != nil {
		return credential.CloudCredentialResult{}, errors.Capture(err)
	}
	if len(creds) > 1 {
		return credential.CloudCredentialResult{}, errors.Errorf("expected 1 credential, got %d", len(creds))
	}
	return creds[0], errors.Capture(err)
}

// GetCloudCredential is responsible for returning a cloud credential identified
// by id. If no cloud credential exists for the given id then a
// [credentialerrors.NotFound] error will be returned.
func (st *State) GetCloudCredential(
	ctx context.Context,
	id corecredential.UUID,
) (credential.CloudCredentialResult, error) {
	db, err := st.DB()
	if err != nil {
		return credential.CloudCredentialResult{}, errors.Capture(err)
	}

	var rval credential.CloudCredentialResult
	return rval, db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		rval, err = GetCloudCredential(ctx, st, tx, id)
		return err
	})
}

// GetCloudCredential is responsible for returning a cloud credential identified
// by id. If no cloud credential exists for the given id then a
// [credentialerrors.NotFound] error will be returned.
func GetCloudCredential(
	ctx context.Context,
	st domain.Preparer,
	tx *sqlair.TX,
	id corecredential.UUID,
) (credential.CloudCredentialResult, error) {
	q := `
SELECT ca.* AS &credentialWithAttribute.*
FROM   v_cloud_credential_attribute ca
WHERE  uuid = $M.id
`

	stmt, err := st.Prepare(q, credentialWithAttribute{}, sqlair.M{})
	if err != nil {
		return credential.CloudCredentialResult{}, errors.Capture(err)
	}

	args := sqlair.M{
		"id": id,
	}
	rows := []credentialWithAttribute{}

	err = tx.Query(ctx, stmt, args).GetAll(&rows)
	if errors.Is(err, sql.ErrNoRows) {
		return credential.CloudCredentialResult{}, errors.Errorf("%w for id %q", credentialerrors.NotFound, id)
	} else if err != nil {
		return credential.CloudCredentialResult{}, errors.Errorf("getting cloud credential for id %q: %w", id, err)
	}

	rval := credential.CloudCredentialResult{
		CloudCredentialInfo: credential.CloudCredentialInfo{
			AuthType:      rows[0].AuthType,
			Attributes:    make(map[string]string, len(rows)),
			Revoked:       rows[0].Revoked,
			Label:         rows[0].Name,
			Invalid:       rows[0].Invalid,
			InvalidReason: rows[0].InvalidReason,
		},
		CloudName: rows[0].CloudName,
	}
	for _, row := range rows {
		rval.Attributes[row.AttributeKey] = row.AttributeValue
	}
	return rval, nil
}

// AllCloudCredentialsForOwner returns all cloud credentials stored on the controller
// for a given owner.
func (st *State) AllCloudCredentialsForOwner(ctx context.Context, owner coreuser.Name) (map[corecredential.Key]credential.CloudCredentialResult, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	var (
		dbRows       Credentials
		dbAuthTypes  []authType
		dbCloudNames []dbCloudName
		keyValues    []CredentialAttribute
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		credQuery := `
SELECT cc.* AS &Credential.*,
       auth_type.type AS &authType.type,
       cloud.name AS &dbCloudName.name,
       (cc_attr.key, cc_attr.value) AS (&CredentialAttribute.*)
FROM   cloud_credential cc
       JOIN auth_type ON cc.auth_type_id = auth_type.id
       JOIN cloud ON cc.cloud_uuid = cloud.uuid
	   JOIN user on cc.owner_uuid = user.uuid
       LEFT JOIN cloud_credential_attribute cc_attr ON cc_attr.cloud_credential_uuid = cc.uuid
WHERE  user.removed = false
AND    user.name = $ownerName.name
`
		ownerName := ownerName{
			Name: owner.Name(),
		}
		credStmt, err := st.Prepare(
			credQuery,
			ownerName,
			dbCloudName{},
			Credential{},
			authType{},
			CredentialAttribute{},
		)
		if err != nil {
			return errors.Errorf("preparing select all credentials for owner statement: %w", err)
		}

		err = tx.Query(ctx, credStmt, ownerName).GetAll(&dbRows, &dbCloudNames, &dbAuthTypes, &keyValues)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("loading cloud credentials: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	result := make(map[corecredential.Key]credential.CloudCredentialResult)
	for _, cloudName := range dbCloudNames {
		infos, err := dbRows.ToCloudCredentials(cloudName.Name, dbAuthTypes, keyValues)
		if err != nil {
			return nil, errors.Capture(err)
		}
		for _, info := range infos {
			result[corecredential.Key{
				Cloud: info.CloudName,
				Owner: owner,
				Name:  info.Label,
			}] = info
		}
	}
	if len(result) == 0 {
		return nil, errors.Errorf("cloud credentials for %q %w", owner, coreerrors.NotFound)
	}
	return result, errors.Capture(err)
}

// RemoveCloudCredential removes a cloud credential with the given name, cloud and owner..
func (st *State) RemoveCloudCredential(ctx context.Context, key corecredential.Key) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	credAttrDeleteStmt, err := st.Prepare(`
DELETE
FROM   cloud_credential_attribute
WHERE  cloud_credential_attribute.cloud_credential_uuid = $credentialUUID.uuid
`, credentialUUID{})
	if err != nil {
		return errors.Capture(err)
	}
	credDeleteStmt, err := st.Prepare(`
DELETE
FROM   cloud_credential
WHERE  cloud_credential.uuid = $credentialUUID.uuid
`, credentialUUID{})
	if err != nil {
		return errors.Capture(err)
	}

	updateModelStmt, err := st.Prepare(`
UPDATE model
SET    cloud_credential_uuid = NULL
WHERE  cloud_credential_uuid = $credentialUUID.uuid
`, credentialUUID{})
	if err != nil {
		return errors.Capture(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		uuid, err := st.credentialUUIDForKey(ctx, tx, key)
		if err != nil {
			return errors.Capture(err)
		}

		// Remove the credential from any models using it.
		credUUID := credentialUUID{UUID: uuid.String()}
		err = tx.Query(ctx, updateModelStmt, credUUID).Run()
		if err != nil {
			return errors.Errorf("reseting model credentials: %w", err)
		}

		if err := tx.Query(ctx, credAttrDeleteStmt, credUUID).Run(); err != nil {
			return errors.Errorf("deleting credential attributes: %w", err)
		}
		err = tx.Query(ctx, credDeleteStmt, credUUID).Run()
		if err != nil {
			return errors.Errorf("deleting credential: %w", err)
		}
		return nil
	})
}

// WatchCredential returns a new NotifyWatcher watching for changes to the specified credential.
func (st *State) WatchCredential(
	ctx context.Context,
	getWatcher func(
		ctx context.Context,
		summary string,
		filter eventsource.FilterOption,
		filterOpts ...eventsource.FilterOption,
	) (watcher.NotifyWatcher, error),
	key corecredential.Key,
) (watcher.NotifyWatcher, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	var id corecredential.UUID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		id, err = st.credentialUUIDForKey(ctx, tx, key)
		return errors.Capture(err)
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	result, err := getWatcher(
		ctx,
		fmt.Sprintf("watching credential for %q", id),
		eventsource.PredicateFilter("cloud_credential", changestream.All, eventsource.EqualsPredicate(id.String())),
	)
	if err != nil {
		return result, errors.Errorf("watching credential: %w", err)
	}
	return result, nil
}

// ModelsUsingCloudCredential returns a map of uuid->name for models which use the credential.
func (st *State) ModelsUsingCloudCredential(ctx context.Context, key corecredential.Key) (map[coremodel.UUID]string, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := st.Prepare(`
SELECT &modelNameAndUUID.*
FROM   v_model m
WHERE  m.cloud_credential_uuid = $credentialUUID.uuid
`, credentialUUID{}, modelNameAndUUID{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	result := make(map[coremodel.UUID]string)
	var modelNameAndUUIDs []modelNameAndUUID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		uuid, err := st.credentialUUIDForKey(ctx, tx, key)
		if err != nil {
			return errors.Capture(err)
		}
		credUUID := credentialUUID{UUID: uuid.String()}
		err = tx.Query(ctx, stmt, credUUID).GetAll(&modelNameAndUUIDs)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	for _, m := range modelNameAndUUIDs {
		result[coremodel.UUID(m.UUID)] = m.Name
	}
	return result, nil
}
