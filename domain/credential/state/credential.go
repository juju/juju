// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils/v3"

	"github.com/juju/juju/cloud"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/database"
	"github.com/juju/juju/domain"
	dbcloud "github.com/juju/juju/domain/cloud/state"
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

// UpsertCloudCredential adds or updates a cloud credential with the given tag.
func (st *State) UpsertCloudCredential(ctx context.Context, name, cloud, owner string, credential cloud.Credential) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		// Get the credential UUID - either existing or make a new one.
		// TODO(wallyworld) - implement owner as a FK when users are modelled.
		stmt := `
SELECT  cloud_credential.uuid
FROM    cloud_credential
        INNER JOIN cloud
            ON cloud_credential.cloud_uuid = cloud.uuid
WHERE   cloud.name = ? AND cloud_credential.name = ? AND cloud_credential.owner_uuid = ?
`
		var credentialUUID string
		row := tx.QueryRowContext(ctx, stmt, cloud, name, owner)
		err := row.Scan(&credentialUUID)
		if err != nil && err != sql.ErrNoRows {
			return errors.Trace(err)
		}
		if err != nil {
			if credential.Invalid || credential.InvalidReason != "" {
				return fmt.Errorf("adding invalid credential %w", errors.NotSupported)
			}
			credentialUUID = utils.MustNewUUID().String()
		}

		if err := upsertCredential(ctx, tx, credentialUUID, name, cloud, owner, credential); err != nil {
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

func upsertCredential(ctx context.Context, tx *sql.Tx, credentialUUID string, name, cloud, owner string, credential cloud.Credential) error {
	dbCredential, err := dbCredentialFromCredential(ctx, tx, credentialUUID, name, cloud, owner, credential)
	if err != nil {
		return errors.Trace(err)
	}

	q := `
INSERT INTO cloud_credential (uuid, name, cloud_uuid, auth_type_id, owner_uuid, revoked, invalid, invalid_reason)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(uuid) DO UPDATE SET name=excluded.name,
                                cloud_uuid=excluded.cloud_uuid,
                                auth_type_id=excluded.auth_type_id,
                                owner_uuid=excluded.owner_uuid,
                                revoked=excluded.revoked,
                                invalid=excluded.invalid,
                                invalid_reason=excluded.invalid_reason;`

	_, err = tx.ExecContext(ctx, q,
		dbCredential.ID,
		dbCredential.Name,
		dbCredential.CloudUUID,
		dbCredential.AuthTypeID,
		dbCredential.OwnerUUID,
		dbCredential.Revoked,
		dbCredential.Invalid,
		dbCredential.InvalidReason,
	)
	if database.IsErrConstraintCheck(err) {
		return fmt.Errorf("%w credential name cannot be empty%w", errors.NotValid, errors.Hide(err))
	} else if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func updateCredentialAttributes(ctx context.Context, tx *sql.Tx, credentialUUID string, attr map[string]string) error {
	keyNamesBinds, keyNames := database.MapKeysToPlaceHolder(attr)

	// Delete any keys no longer in the attributes map.
	deleteQuery := fmt.Sprintf(`
DELETE FROM  cloud_credential_attributes
WHERE        cloud_credential_uuid = ?
AND          key NOT IN (%s)
`, keyNamesBinds)

	args := append([]any{credentialUUID}, keyNames...)
	if _, err := tx.ExecContext(ctx, deleteQuery, args...); err != nil {
		return errors.Trace(err)
	}

	insertQuery := `
INSERT INTO cloud_credential_attributes (cloud_credential_uuid, key, value)
VALUES (?, ?, ?)
ON CONFLICT(cloud_credential_uuid, key) DO UPDATE SET key=excluded.key,
                                                      value=excluded.value
`
	for key, value := range attr {
		if _, err := tx.ExecContext(ctx, insertQuery, credentialUUID, key, value); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func dbCredentialFromCredential(ctx context.Context, tx *sql.Tx, credentialUUID string, name, cloud, owner string, credential cloud.Credential) (*CloudCredential, error) {
	cred := &CloudCredential{
		ID:         credentialUUID,
		Name:       name,
		AuthTypeID: -1,
		// TODO(wallyworld) - implement owner as a FK when users are modelled.
		OwnerUUID:     owner,
		Revoked:       credential.Revoked,
		Invalid:       credential.Invalid,
		InvalidReason: credential.InvalidReason,
	}

	row := tx.QueryRowContext(ctx, "SELECT uuid FROM cloud WHERE name = ?", cloud)
	err := row.Scan(&cred.CloudUUID)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("cloud %q %w", cloud, errors.NotValid)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}

	validAuthTypes, err := validAuthTypesForCloud(ctx, tx, cloud)
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
		return nil, fmt.Errorf("validating credential %q owned by %q for cloud %q: supported auth-types %q, %q %w", name, owner, cloud, validAuthTypeNames, credential.AuthType(), errors.NotSupported)
	}
	return cred, nil
}

func validAuthTypesForCloud(ctx context.Context, tx *sql.Tx, cloudName string) ([]dbcloud.AuthType, error) {
	authTypeQuery := `
SELECT  auth_type.id, auth_type.type
FROM    auth_type
WHERE auth_type.id in (
    SELECT auth_type_id
    FROM   cloud_auth_type
    INNER JOIN cloud
          ON cloud_auth_type.cloud_uuid = cloud.uuid
    WHERE cloud.name = ?
);`
	rows, err := tx.QueryContext(ctx, authTypeQuery, cloudName)
	if err != nil && err != sql.ErrNoRows {
		return nil, errors.Trace(err)
	}
	defer func() { _ = rows.Close() }()

	var result []dbcloud.AuthType
	for rows.Next() {
		var authType dbcloud.AuthType
		if err := rows.Scan(&authType.ID, &authType.Type); err != nil {
			return nil, errors.Trace(err)
		}
		result = append(result, authType)
	}
	if err := rows.Err(); err != nil {
		return nil, errors.Trace(err)
	}
	return result, errors.Trace(err)
}

// InvalidateCloudCredential marks a cloud credential with the given tag as invalid.
func (st *State) InvalidateCloudCredential(ctx context.Context, name, cloudName, owner, reason string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		q := `
UPDATE cloud_credential
SET    invalid = true, invalid_reason = ?
WHERE  cloud_credential.name = ?
AND    cloud_credential.owner_uuid = ?
AND    cloud_credential.cloud_uuid = (
    SELECT uuid FROM cloud
    WHERE name = ?
);`
		r, err := tx.ExecContext(ctx, q, reason, name, owner, cloudName)
		if err != nil {
			return errors.Trace(err)
		}
		n, err := r.RowsAffected()
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

	var creds []cloud.Credential
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		creds, err = st.loadCloudCredentials(ctx, tx, "", cloudName, owner)
		return errors.Trace(err)
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := make(map[string]cloud.Credential)
	for _, cred := range creds {
		result[cred.Label] = cred
	}
	return result, nil
}

// CloudCredential returns the cloud credential for the given tag.
func (st *State) CloudCredential(ctx context.Context, name, cloudName, owner string) (cloud.Credential, error) {
	db, err := st.DB()
	if err != nil {
		return cloud.Credential{}, errors.Trace(err)
	}

	var creds []cloud.Credential
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
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
	return creds[0], errors.Trace(err)
}

type credentialInfo struct {
	id            string
	name          string
	revoked       bool
	invalid       bool
	invalidReason string
	authType      string
	cloudName     string
	owner         string
}

type credentialAttrs map[string]string

func (st *State) loadCloudCredentials(ctx context.Context, tx *sql.Tx, name, cloudName, owner string) ([]cloud.Credential, error) {
	// First load the basic credential info.
	credQuery := `
SELECT cloud_credential.uuid, cloud_credential.name,
       cloud_credential.revoked, cloud_credential.invalid, 
       cloud_credential.invalid_reason, 
       cloud_credential.owner_uuid AS owner,
       auth_type.type AS auth_type, cloud.name AS cloud_name,
       cloud_credential_attributes.key, cloud_credential_attributes.value
FROM   cloud_credential
       INNER JOIN auth_type 
             ON cloud_credential.auth_type_id = auth_type.id
       INNER JOIN cloud 
             ON cloud_credential.cloud_uuid = cloud.uuid
       LEFT JOIN cloud_credential_attributes
            ON cloud_credential_attributes.cloud_credential_uuid = cloud_credential.uuid
`

	var (
		args        []any
		filterTerms []string
	)
	if name != "" {
		filterTerms = append(filterTerms, "cloud_credential.name = ?")
		args = append(args, name)
	}
	if cloudName != "" {
		filterTerms = append(filterTerms, "cloud.name = ?")
		args = append(args, cloudName)
	}
	if owner != "" {
		filterTerms = append(filterTerms, "cloud_credential.owner_uuid = ?")
		args = append(args, owner)
	}
	if len(filterTerms) > 0 {
		credQuery = credQuery + "\nWHERE " + strings.Join(filterTerms, " AND ")
	}

	rows, err := tx.QueryContext(ctx, credQuery, args...)
	if err != nil && err != sql.ErrNoRows {
		return nil, errors.Trace(err)
	}
	defer func() { _ = rows.Close() }()

	var result []cloud.Credential
	recordResult := func(info *credentialInfo, attrs credentialAttrs) {
		cred := cloud.NewNamedCredential(info.name, cloud.AuthType(info.authType), attrs, info.revoked)
		cred.Invalid = info.invalid
		cred.InvalidReason = info.invalidReason
		result = append(result, cred)
	}

	var (
		current *credentialInfo
		attrs   = make(credentialAttrs)
	)
	for rows.Next() {
		var (
			dbCredential credentialInfo
			key, value   string
		)
		if err := rows.Scan(
			&dbCredential.id, &dbCredential.name, &dbCredential.revoked, &dbCredential.invalid, &dbCredential.invalidReason,
			&dbCredential.owner, &dbCredential.authType, &dbCredential.cloudName,
			&key, &value,
		); err != nil {
			return nil, errors.Trace(err)
		}
		if current != nil && dbCredential.id != current.id {
			recordResult(current, attrs)
			attrs = make(credentialAttrs)
		}
		attrs[key] = value
		current = &dbCredential
	}
	if current != nil {
		recordResult(current, attrs)
	}
	if err := rows.Err(); err != nil {
		return nil, errors.Trace(err)
	}
	return result, nil
}

// AllCloudCredentials returns all cloud credentials stored on the controller
// for a given user.
func (st *State) AllCloudCredentials(ctx context.Context, owner string) ([]cloud.Credential, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var result []cloud.Credential
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
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

// RemoveCloudCredential removes a cloud credential with the given tag.
func (st *State) RemoveCloudCredential(ctx context.Context, name, cloudName, owner string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	selectStmt := `
SELECT uuid FROM cloud_credential
WHERE  cloud_credential.name = ?
AND    cloud_credential.owner_uuid = ?
AND    cloud_credential.cloud_uuid = (
    SELECT uuid FROM cloud
    WHERE name = ?
);`

	credAttrDelete := `
DELETE FROM cloud_credential_attributes
WHERE  cloud_credential_attributes.cloud_credential_uuid = ?;
`

	credDelete := `
DELETE FROM cloud_credential
WHERE  cloud_credential.uuid = ?
;`

	return db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		var uuid string
		row := tx.QueryRowContext(ctx, selectStmt, name, owner, cloudName)
		if err := row.Scan(&uuid); err == sql.ErrNoRows {
			return fmt.Errorf("cloud credential %s/%s/%s %w%w", cloudName, owner, name, errors.NotFound, errors.Hide(err))
		} else if err != nil {
			return fmt.Errorf("fetching cloud credential %s/%s/%s: %w", cloudName, owner, name, err)
		}

		if _, err := tx.ExecContext(ctx, credAttrDelete, uuid); err != nil {
			return errors.Annotate(err, "deleting credential attributes")
		}
		_, err = tx.ExecContext(ctx, credDelete, uuid)
		return errors.Annotate(err, "deleting credential")
	})
}

// TODO(wallyworld) - implement the following methods
// once users and permissions are modelled.

// CredentialOwnerModelAccess stores cloud credential model information for the credential owner
// or an error retrieving it.
type CredentialOwnerModelAccess struct {
	ModelUUID   string
	ModelName   string
	OwnerAccess permission.Access
	Error       error
}

// CredentialModelsAndOwnerAccess returns all models that use given cloud credential as well as
// what access the credential owner has on these models.
func (st *State) CredentialModelsAndOwnerAccess(ctx context.Context, name, cloudName, owner string) ([]CredentialOwnerModelAccess, error) {
	return nil, nil
}

// RemoveModelsCredential clears out given credential reference from all models that have it.
func (st *State) RemoveModelsCredential(ctx context.Context, name, cloudName, owner string) error {
	return nil
}

// TODO(wallyworld) - implement once watcher supports composite keys

// WatchCredential returns a new NotifyWatcher watching for
// changes to the specified credential.
func (st *State) WatchCredential(name, cloudName, owner string) watcher.NotifyWatcher {
	return nil
}
