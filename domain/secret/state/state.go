// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	coredatabase "github.com/juju/juju/core/database"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/domain"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	modelerrors "github.com/juju/juju/domain/model/errors"
	domainsecret "github.com/juju/juju/domain/secret"
	secreterrors "github.com/juju/juju/domain/secret/errors"
	uniterrors "github.com/juju/juju/domain/unit/errors"
	"github.com/juju/juju/internal/uuid"
)

// State represents database interactions dealing with storage pools.
type State struct {
	*domain.StateBase
}

// NewState returns a new secretMetadata state
// based on the input database factory method.
func NewState(factory coredatabase.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}

// GetModelUUID returns the uuid of the model,
// or an error satisfying [modelerrors.NotFound]
func (st State) GetModelUUID(ctx context.Context) (string, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Trace(err)
	}

	getModelUUIDSQL := "SELECT &M.uuid FROM model"
	getModelUUIDStmt, err := st.Prepare(getModelUUIDSQL, sqlair.M{})
	if err != nil {
		return "", errors.Trace(err)
	}
	var modelUUID string
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		result := sqlair.M{}
		err = tx.Query(ctx, getModelUUIDStmt).Get(&result)
		if err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return modelerrors.NotFound
			} else {
				return errors.Annotatef(err, "looking up model UUID")
			}
		}
		modelUUID = result["uuid"].(string)
		return nil
	})
	return modelUUID, errors.Trace(domain.CoerceError(err))
}

// CreateUserSecret creates a user secret, returning an error satisfying [secreterrors.SecretAlreadyExists]
// if a user secret with the same label already exists.
func (st State) CreateUserSecret(ctx context.Context, version int, uri *coresecrets.URI, secret domainsecret.UpsertSecretParams) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	revisionUUID, err := uuid.NewUUID()
	if err != nil {
		return errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := st.createSecret(ctx, tx, version, uri, secret, revisionUUID, st.checkUserSecretLabelExists); err != nil {
			return errors.Annotatef(err, "inserting secret records for secret %q", uri)
		}

		dbSecretOwner := secretModelOwner{SecretID: uri.ID, Label: secret.Label}
		if err := st.upsertSecretModelOwner(ctx, tx, dbSecretOwner); err != nil {
			return errors.Annotatef(err, "inserting user secret record for secret %q", uri)
		}
		return nil
	})
	return errors.Trace(domain.CoerceError(err))
}

// checkSecretUserLabelExists returns an error if a user secret with the given label already exists.
func (st State) checkUserSecretLabelExists(ctx context.Context, tx *sqlair.TX, label string) error {
	checkLabelExistsSQL := `
SELECT &secretOwner.secret_id
FROM   secret_model_owner
WHERE  label = $secretOwner.label`
	checkExistsStmt, err := st.Prepare(checkLabelExistsSQL, secretOwner{})
	if err != nil {
		return errors.Trace(err)
	}
	dbSecretOwner := secretOwner{Label: label}
	err = tx.Query(ctx, checkExistsStmt, dbSecretOwner).Get(&dbSecretOwner)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Trace(domain.CoerceError(err))
	}
	if err == nil {
		return fmt.Errorf("secret with label %q already exists%w", label, errors.Hide(secreterrors.SecretLabelAlreadyExists))
	}
	return nil
}

// CreateCharmApplicationSecret creates a secret onwed by the specified application,
// returning an error satisfying [secreterrors.SecretAlreadyExists] if a secret
// owned by the same application with the same label already exists.
// It also returns an error satisfying [applicationerrors.ApplicationNotFound] if
// the application does not exist.
func (st State) CreateCharmApplicationSecret(ctx context.Context, version int, uri *coresecrets.URI, appName string, secret domainsecret.UpsertSecretParams) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	revisionUUID, err := uuid.NewUUID()
	if err != nil {
		return errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		dbSecretOwner := secretApplicationOwner{SecretID: uri.ID, Label: secret.Label}

		selectApplicationUUID := `SELECT &M.uuid FROM application WHERE name=$M.name`
		selectApplicationUUIDStmt, err := st.Prepare(selectApplicationUUID, sqlair.M{})
		if err != nil {
			return errors.Trace(err)
		}

		result := sqlair.M{}
		err = tx.Query(ctx, selectApplicationUUIDStmt, sqlair.M{"name": appName}).Get(&result)
		if err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return applicationerrors.ApplicationNotFound
			} else {
				return errors.Annotatef(err, "looking up napplication UUID for %q", appName)
			}
		}
		dbSecretOwner.ApplicationUUID = result["uuid"].(string)

		if err := st.createSecret(ctx, tx, version, uri, secret, revisionUUID, st.checkApplicationSecretLabelExists(appName, dbSecretOwner.ApplicationUUID)); err != nil {
			return errors.Annotatef(err, "inserting secret records for secret %q", uri)
		}

		if err := st.upsertSecretApplicationOwner(ctx, tx, dbSecretOwner); err != nil {
			return errors.Annotatef(err, "inserting application secret record for secret %q", uri)
		}
		return nil
	})
	return errors.Trace(domain.CoerceError(err))
}

// checkApplicationSecretLabelExists returns function which checks if a charm application secret with the given label already exists.
func (st State) checkApplicationSecretLabelExists(appName, app_uuid string) checkExistsFunc {
	return func(ctx context.Context, tx *sqlair.TX, label string) error {
		if label == "" {
			return nil
		}

		checkLabelExistsSQL := `
SELECT &secretApplicationOwner.secret_id
FROM   secret_application_owner
WHERE  label = $secretApplicationOwner.label
AND    application_uuid = $secretApplicationOwner.application_uuid`

		checkExistsStmt, err := st.Prepare(checkLabelExistsSQL, secretApplicationOwner{})
		if err != nil {
			return errors.Trace(err)
		}
		dbSecretOwner := secretApplicationOwner{Label: label, ApplicationUUID: app_uuid}
		err = tx.Query(ctx, checkExistsStmt, dbSecretOwner).Get(&dbSecretOwner)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Trace(domain.CoerceError(err))
		}
		if err == nil {
			return fmt.Errorf("secret with label %q already exists for application %q%w", label, appName, errors.Hide(secreterrors.SecretLabelAlreadyExists))
		}
		return nil
	}
}

// CreateCharmUnitSecret creates a secret onwed by the specified unit,
// returning an error satisfying [secreterrors.SecretAlreadyExists] if a secret
// owned by the same unit with the same label already exists.
// It also returns an error satisfying [uniterrors.NotFound] if
// the unit does not exist.
func (st State) CreateCharmUnitSecret(ctx context.Context, version int, uri *coresecrets.URI, unitName string, secret domainsecret.UpsertSecretParams) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	revisionUUID, err := uuid.NewUUID()
	if err != nil {
		return errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		dbSecretOwner := secretUnitOwner{SecretID: uri.ID, Label: secret.Label}

		selectUnitUUID := `SELECT &M.uuid FROM unit WHERE unit_id=$M.unit_id`
		selectUnitUUIDStmt, err := st.Prepare(selectUnitUUID, sqlair.M{})
		if err != nil {
			return errors.Trace(err)
		}

		result := sqlair.M{}
		err = tx.Query(ctx, selectUnitUUIDStmt, sqlair.M{"unit_id": unitName}).Get(&result)
		if err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return uniterrors.NotFound
			} else {
				return errors.Annotatef(err, "looking up unit UUID for %q", unitName)
			}
		}
		dbSecretOwner.UnitUUID = result["uuid"].(string)

		if err := st.createSecret(ctx, tx, version, uri, secret, revisionUUID, st.checkUnitSecretLabelExists(unitName, dbSecretOwner.UnitUUID)); err != nil {
			return errors.Annotatef(err, "inserting secret records for secret %q", uri)
		}

		if err := st.upsertSecretUnitOwner(ctx, tx, dbSecretOwner); err != nil {
			return errors.Annotatef(err, "inserting unit secret record for secret %q", uri)
		}
		return nil
	})
	return errors.Trace(domain.CoerceError(err))
}

// checkUnitSecretLabelExists returns function which checks if a charm unit secret with the given label already exists.
func (st State) checkUnitSecretLabelExists(unitName, unit_uuid string) checkExistsFunc {
	return func(ctx context.Context, tx *sqlair.TX, label string) error {
		if label == "" {
			return nil
		}

		checkLabelExistsSQL := `
SELECT &secretUnitOwner.secret_id
FROM   secret_unit_owner
WHERE  label = $secretUnitOwner.label
AND    unit_uuid = $secretUnitOwner.unit_uuid`

		checkExistsStmt, err := st.Prepare(checkLabelExistsSQL, secretUnitOwner{})
		if err != nil {
			return errors.Trace(err)
		}
		dbSecretOwner := secretUnitOwner{Label: label, UnitUUID: unit_uuid}
		err = tx.Query(ctx, checkExistsStmt, dbSecretOwner).Get(&dbSecretOwner)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Trace(domain.CoerceError(err))
		}
		if err == nil {
			return fmt.Errorf("secret with label %q already exists for unit %q%w", label, unitName, errors.Hide(secreterrors.SecretLabelAlreadyExists))
		}
		return nil
	}
}

type checkExistsFunc = func(ctx context.Context, tx *sqlair.TX, label string) error

// createSecret creates the records needed to store secret data, excluding secret owner records.
func (st State) createSecret(
	ctx context.Context, tx *sqlair.TX, version int, uri *coresecrets.URI,
	secret domainsecret.UpsertSecretParams, revisionUUID uuid.UUID,
	checkExists checkExistsFunc,
) error {
	if len(secret.Data) == 0 && secret.ValueRef == nil {
		return errors.Errorf("cannot create a secret without content")
	}
	if secret.Label != "" {
		if err := checkExists(ctx, tx, secret.Label); err != nil {
			return errors.Trace(err)
		}
	}

	now := time.Now()
	dbSecret := secretMetadata{
		ID:          uri.ID,
		Version:     version,
		Description: secret.Description,
		AutoPrune:   secret.AutoPrune,
		CreateTime:  now,
		UpdateTime:  now,
	}
	if err := st.upsertSecret(ctx, tx, dbSecret); err != nil {
		return errors.Annotatef(err, "creating user secret %q", uri)
	}

	dbRevision := secretRevision{
		ID:         revisionUUID.String(),
		SecretID:   uri.ID,
		Revision:   1,
		CreateTime: now,
		UpdateTime: now,
	}
	if err := st.upsertSecretRevision(ctx, tx, dbRevision); err != nil {
		return errors.Annotatef(err, "inserting revision for secret %q", uri)
	}

	if len(secret.Data) > 0 {
		if err := st.updateSecretContent(ctx, tx, dbRevision.ID, secret.Data); err != nil {
			return errors.Annotatef(err, "updating content for secret %q", uri)
		}
	}

	if secret.ValueRef != nil {
		if err := st.upsertSecretValueRef(ctx, tx, dbRevision.ID, secret.ValueRef); err != nil {
			return errors.Annotatef(err, "updating backend value reference for secret %q", uri)
		}
	}
	return nil
}

func (st State) upsertSecret(ctx context.Context, tx *sqlair.TX, dbSecret secretMetadata) error {
	insertQuery := `
INSERT INTO secret (*)
VALUES ($secretMetadata.*)
ON CONFLICT(id) DO UPDATE SET 
    version=excluded.version,
    description=excluded.description,
    rotate_policy=excluded.rotate_policy,
    auto_prune=excluded.auto_prune,
    update_time=excluded.update_time
`

	insertStmt, err := st.Prepare(insertQuery, secretMetadata{})
	if err != nil {
		return errors.Trace(err)
	}

	err = tx.Query(ctx, insertStmt, dbSecret).Run()
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (st State) upsertSecretModelOwner(ctx context.Context, tx *sqlair.TX, owner secretModelOwner) error {
	insertQuery := `
INSERT INTO secret_model_owner (secret_id, label)
VALUES      ($secretModelOwner.*)
ON CONFLICT(secret_id) DO UPDATE SET label=excluded.label
`

	insertStmt, err := st.Prepare(insertQuery, secretModelOwner{})
	if err != nil {
		return errors.Trace(err)
	}

	err = tx.Query(ctx, insertStmt, owner).Run()
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (st State) upsertSecretApplicationOwner(ctx context.Context, tx *sqlair.TX, owner secretApplicationOwner) error {
	insertQuery := `
INSERT INTO secret_application_owner (secret_id, application_uuid, label)
VALUES      ($secretApplicationOwner.*)
ON CONFLICT(secret_id, application_uuid) DO UPDATE SET label=excluded.label
`

	insertStmt, err := st.Prepare(insertQuery, secretApplicationOwner{})
	if err != nil {
		return errors.Trace(err)
	}

	err = tx.Query(ctx, insertStmt, owner).Run()
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (st State) upsertSecretUnitOwner(ctx context.Context, tx *sqlair.TX, owner secretUnitOwner) error {
	insertQuery := `
INSERT INTO secret_unit_owner (secret_id, unit_uuid, label)
VALUES      ($secretUnitOwner.*)
ON CONFLICT(secret_id, unit_uuid) DO UPDATE SET label=excluded.label
`

	insertStmt, err := st.Prepare(insertQuery, secretUnitOwner{})
	if err != nil {
		return errors.Trace(err)
	}

	err = tx.Query(ctx, insertStmt, owner).Run()
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (st State) upsertSecretRevision(ctx context.Context, tx *sqlair.TX, dbRevision secretRevision) error {
	insertQuery := `
INSERT INTO secret_revision (*)
VALUES ($secretRevision.*)
ON CONFLICT(uuid) DO UPDATE SET
    obsolete=excluded.obsolete,
    pending_delete=excluded.pending_delete,
    update_time=excluded.update_time
`

	insertStmt, err := st.Prepare(insertQuery, secretRevision{})
	if err != nil {
		return errors.Trace(err)
	}

	err = tx.Query(ctx, insertStmt, dbRevision).Run()
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (st State) upsertSecretValueRef(ctx context.Context, tx *sqlair.TX, revisionUUID string, valueRef *coresecrets.ValueRef) error {
	insertQuery := `
INSERT INTO secret_value_ref (*)
VALUES ($secretValueRef.*)
ON CONFLICT(revision_uuid) DO UPDATE SET
    backend_uuid=excluded.backend_uuid,
    revision_id=excluded.revision_id
`

	insertStmt, err := st.Prepare(insertQuery, secretValueRef{})
	if err != nil {
		return errors.Trace(err)
	}

	err = tx.Query(ctx, insertStmt, secretValueRef{
		RevisionUUID: revisionUUID,
		BackendUUID:  valueRef.BackendID,
		RevisionID:   valueRef.RevisionID,
	}).Run()
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

type keysToKeep []string

func (st State) updateSecretContent(ctx context.Context, tx *sqlair.TX, revisionUUID string, content coresecrets.SecretData) error {
	// Delete any keys no longer in the content map.
	deleteQuery := fmt.Sprintf(`
DELETE FROM  secret_content
WHERE        revision_uuid = $M.id
AND          name NOT IN ($keysToKeep[:])
`)

	deleteStmt, err := st.Prepare(deleteQuery, sqlair.M{}, keysToKeep{})
	if err != nil {
		return errors.Trace(err)
	}

	insertQuery := `
INSERT INTO secret_content
VALUES (
    $secretContent.revision_uuid,
    $secretContent.name,
    $secretContent.content
)
ON CONFLICT(revision_uuid, name) DO UPDATE SET
    name=excluded.name,
    content=excluded.content
`
	insertStmt, err := st.Prepare(insertQuery, secretContent{})
	if err != nil {
		return errors.Trace(err)
	}

	var keys keysToKeep
	for k := range content {
		keys = append(keys, k)
	}
	if err := tx.Query(ctx, deleteStmt, sqlair.M{"id": revisionUUID}, keys).Run(); err != nil {
		return errors.Trace(err)
	}
	for key, value := range content {
		if err := tx.Query(ctx, insertStmt, secretContent{
			RevisionUUID: revisionUUID,
			Name:         key,
			Content:      value,
		}).Run(); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// ListSecrets returns the secrets matching the specified criteria.
// If all terms are empty, then all secrets are returned.
func (st State) ListSecrets(ctx context.Context, uri *coresecrets.URI,
	revision *int,
	// TODO(secrets) - use all filter terms
	labels domainsecret.Labels,
) ([]*coresecrets.SecretMetadata, [][]*coresecrets.SecretRevisionMetadata, error) {
	db, err := st.DB()
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	var (
		secrets        []*coresecrets.SecretMetadata
		revisionResult [][]*coresecrets.SecretRevisionMetadata
	)
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		secrets, err = st.listSecretsAnyOwner(ctx, tx, uri)
		if err != nil {
			return errors.Annotate(err, "querying secrets")
		}
		revisionResult = make([][]*coresecrets.SecretRevisionMetadata, len(secrets))
		for i, secret := range secrets {
			secretRevisions, err := st.listSecretRevisions(ctx, tx, secret.URI, revision)
			if err != nil {
				return errors.Annotatef(err, "querying secret revisions for %q", secret.URI.ID)
			}
			revisionResult[i] = secretRevisions
		}
		return nil
	}); err != nil {
		return nil, nil, errors.Trace(domain.CoerceError(err))
	}

	return secrets, revisionResult, nil
}

// GetSecret returns the secret with the given URI, returning an error satisfying [secreterrors.SecretNotFound]
// if the secret does not exist.
// TODO(secrets) - fill in Access etc
func (st State) GetSecret(ctx context.Context, uri *coresecrets.URI) (*coresecrets.SecretMetadata, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var secrets []*coresecrets.SecretMetadata
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		secrets, err = st.listSecretsAnyOwner(ctx, tx, uri)
		return errors.Annotatef(err, "querying secret for %q", uri.ID)
	}); err != nil {
		return nil, errors.Trace(domain.CoerceError(err))
	}

	if len(secrets) == 0 {
		return nil, fmt.Errorf("secret %q not found%w", uri, errors.Hide(secreterrors.SecretNotFound))
	}
	return secrets[0], nil
}

func (st State) listSecretsAnyOwner(
	ctx context.Context, tx *sqlair.TX, uri *coresecrets.URI,
) ([]*coresecrets.SecretMetadata, error) {

	query := fmt.Sprintf(`
WITH rev AS
    (SELECT  secret_id, MAX(revision) AS latest_revision
    FROM     secret_revision
    GROUP BY secret_id)
SELECT 
     id AS &secretInfo.id,
     version as &secretInfo.version,
     description as &secretInfo.description,
     auto_prune as &secretInfo.auto_prune,
     create_time as &secretInfo.create_time,
     update_time as &secretInfo.update_time,
     rev.latest_revision AS &secretInfo.latest_revision,
     so.owner_kind AS &secretOwner.owner_kind,
     so.owner_id AS &secretOwner.owner_id,
     so.label AS &secretOwner.label
FROM secret
       JOIN rev ON rev.secret_id = secret.id
       LEFT JOIN (
          SELECT '%s' AS owner_kind, (SELECT uuid FROM model) AS owner_id, label, secret_id
          FROM   secret_model_owner so
       UNION
          SELECT '%s' AS owner_kind, application.name AS owner_id, label, secret_id
          FROM   secret_application_owner so
          JOIN   application
          WHERE  application.uuid = so.application_uuid
       UNION
          SELECT '%s' AS owner_kind, unit.unit_id AS owner_id, label, secret_id
          FROM   secret_unit_owner so
          JOIN   unit
          WHERE  unit.uuid = so.unit_uuid
       ) so ON so.secret_id = secret.id
`, coresecrets.ModelOwner, coresecrets.ApplicationOwner, coresecrets.UnitOwner)

	queryTypes := []any{
		secretInfo{},
		secretOwner{},
	}
	queryParams := []any{}
	if uri != nil {
		queryTypes = append(queryTypes, sqlair.M{})
		query = query + "\nWHERE secret.id = $M.id"
		queryParams = append(queryParams, sqlair.M{"id": uri.ID})
	}

	queryStmt, err := st.Prepare(query, queryTypes...)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var (
		dbSecrets      secrets
		dbsecretOwners []secretOwner
	)
	err = tx.Query(ctx, queryStmt, queryParams...).GetAll(&dbSecrets, &dbsecretOwners)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Trace(err)
	}
	return dbSecrets.toSecretMetadata(dbsecretOwners)
}

// ListCharmSecrets returns charm secrets owned by the specified applications and/or units.
// At least one owner must be specified.
func (st State) ListCharmSecrets(ctx context.Context,
	appOwners domainsecret.ApplicationOwners, unitOwners domainsecret.UnitOwners,
) ([]*coresecrets.SecretMetadata, [][]*coresecrets.SecretRevisionMetadata, error) {
	db, err := st.DB()
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	var (
		secrets        []*coresecrets.SecretMetadata
		revisionResult [][]*coresecrets.SecretRevisionMetadata
	)
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		secrets, err = st.listCharmSecrets(ctx, tx, appOwners, unitOwners)
		if err != nil {
			return errors.Annotate(err, "querying charm secrets")
		}
		revisionResult = make([][]*coresecrets.SecretRevisionMetadata, len(secrets))
		for i, secret := range secrets {
			secretRevisions, err := st.listSecretRevisions(ctx, tx, secret.URI, nil)
			if err != nil {
				return errors.Annotatef(err, "querying secret revisions for %q", secret.URI.ID)
			}
			revisionResult[i] = secretRevisions
		}
		return nil
	}); err != nil {
		return nil, nil, errors.Trace(domain.CoerceError(err))
	}

	return secrets, revisionResult, nil
}

func (st State) listCharmSecrets(
	ctx context.Context, tx *sqlair.TX,
	appOwners domainsecret.ApplicationOwners, unitOwners domainsecret.UnitOwners,
) ([]*coresecrets.SecretMetadata, error) {
	if len(appOwners) == 0 && len(unitOwners) == 0 {
		return nil, errors.New("must supply at least one app owner or unit owner")
	}

	preQueryParts := []string{`
WITH rev AS
    (SELECT  secret_id, MAX(revision) AS latest_revision
    FROM     secret_revision
    GROUP BY secret_id)`[1:]}

	appOwnerSelect := fmt.Sprintf(`
app_owners AS
    (SELECT '%s' AS owner_kind, application.name AS owner_id, label, secret_id
     FROM   secret_application_owner so
     JOIN   application
     WHERE  application.uuid = so.application_uuid
     AND application.name IN ($ApplicationOwners[:]))`[1:], coresecrets.ApplicationOwner)

	unitOwnerSelect := fmt.Sprintf(`
unit_owners AS
    (SELECT '%s' AS owner_kind, unit.unit_id AS owner_id, label, secret_id
     FROM   secret_unit_owner so
     JOIN   unit
     WHERE  unit.uuid = so.unit_uuid
     AND unit.unit_id IN ($UnitOwners[:]))`[1:], coresecrets.UnitOwner)

	if len(appOwners) > 0 {
		preQueryParts = append(preQueryParts, appOwnerSelect)
	}
	if len(unitOwners) > 0 {
		preQueryParts = append(preQueryParts, unitOwnerSelect)
	}
	queryParts := []string{strings.Join(preQueryParts, ",\n")}

	query := `
SELECT 
     id AS &secretInfo.id,
     version as &secretInfo.version,
     description as &secretInfo.description,
     auto_prune as &secretInfo.auto_prune,
     create_time as &secretInfo.create_time,
     update_time as &secretInfo.update_time,
     rev.latest_revision AS &secretInfo.latest_revision,
     so.owner_kind AS &secretOwner.owner_kind,
     so.owner_id AS &secretOwner.owner_id,
     so.label AS &secretOwner.label
FROM secret
   JOIN rev ON rev.secret_id = secret.id`[1:]

	queryParts = append(queryParts, query)

	queryTypes := []any{
		secretInfo{},
		secretOwner{},
	}

	queryParams := []any{}
	var ownerParts []string
	if len(appOwners) > 0 {
		ownerParts = append(ownerParts, "SELECT * FROM app_owners")
		queryTypes = append(queryTypes, domainsecret.ApplicationOwners{})
		queryParams = append(queryParams, appOwners)
	}
	if len(unitOwners) > 0 {
		ownerParts = append(ownerParts, "SELECT * FROM unit_owners")
		queryTypes = append(queryTypes, domainsecret.UnitOwners{})
		queryParams = append(queryParams, unitOwners)
	}
	ownerJoin := fmt.Sprintf(`
    JOIN (
      %s
    ) so ON so.secret_id = secret.id
`[1:], strings.Join(ownerParts, "\nUNION\n"))

	queryParts = append(queryParts, ownerJoin)

	queryStmt, err := st.Prepare(strings.Join(queryParts, "\n"), queryTypes...)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var (
		dbSecrets      secrets
		dbsecretOwners []secretOwner
	)
	err = tx.Query(ctx, queryStmt, queryParams...).GetAll(&dbSecrets, &dbsecretOwners)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Trace(err)
	}
	return dbSecrets.toSecretMetadata(dbsecretOwners)
}

// ListUserSecrets returns all of the user secrets.
func (st State) ListUserSecrets(ctx context.Context) ([]*coresecrets.SecretMetadata, [][]*coresecrets.SecretRevisionMetadata, error) {
	db, err := st.DB()
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	var (
		secrets        []*coresecrets.SecretMetadata
		revisionResult [][]*coresecrets.SecretRevisionMetadata
	)
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		secrets, err = st.listUserSecrets(ctx, tx)
		if err != nil {
			return errors.Annotate(err, "querying user secrets")
		}
		revisionResult = make([][]*coresecrets.SecretRevisionMetadata, len(secrets))
		for i, secret := range secrets {
			secretRevisions, err := st.listSecretRevisions(ctx, tx, secret.URI, nil)
			if err != nil {
				return errors.Annotatef(err, "querying secret revisions for %q", secret.URI.ID)
			}
			revisionResult[i] = secretRevisions
		}
		return nil
	}); err != nil {
		return nil, nil, errors.Trace(domain.CoerceError(err))
	}

	return secrets, revisionResult, nil
}

func (st State) listUserSecrets(
	ctx context.Context, tx *sqlair.TX,
) ([]*coresecrets.SecretMetadata, error) {
	query := fmt.Sprintf(`
WITH rev AS
    (SELECT  secret_id, MAX(revision) AS latest_revision
    FROM     secret_revision
    GROUP BY secret_id)
SELECT 
     id AS &secretInfo.id,
     version as &secretInfo.version,
     description as &secretInfo.description,
     auto_prune as &secretInfo.auto_prune,
     create_time as &secretInfo.create_time,
     update_time as &secretInfo.update_time,
     rev.latest_revision AS &secretInfo.latest_revision,
     so.owner_kind AS &secretOwner.owner_kind,
     so.owner_id AS &secretOwner.owner_id,
     so.label AS &secretOwner.label
FROM secret
       JOIN rev ON rev.secret_id = secret.id
       JOIN (
          SELECT '%s' AS owner_kind, (SELECT uuid FROM model) AS owner_id, label, secret_id
          FROM   secret_model_owner
       ) so ON so.secret_id = secret.id
`, coresecrets.ModelOwner)

	queryStmt, err := st.Prepare(query, secretInfo{}, secretOwner{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	var (
		dbSecrets      secrets
		dbsecretOwners []secretOwner
	)
	err = tx.Query(ctx, queryStmt).GetAll(&dbSecrets, &dbsecretOwners)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Trace(err)
	}
	return dbSecrets.toSecretMetadata(dbsecretOwners)
}

// GetUserSecretURIByLabel returns the URI for the user secret with the specified label,
// or an error satisfying [secreterrors.SecretNotFound] if the secret does not exist.
func (st State) GetUserSecretURIByLabel(ctx context.Context, label string) (*coresecrets.URI, error) {
	if label == "" {
		return nil, errors.NotValidf("empty secret label")
	}

	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	query := `	
SELECT id AS &secretInfo.id
FROM   secret
JOIN   secret_model_owner mso ON secret.id = mso.secret_id
WHERE  mso.label = $M.label
	`

	queryStmt, err := st.Prepare(query, secretInfo{}, sqlair.M{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	var dbSecrets secrets
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		err = tx.Query(ctx, queryStmt, sqlair.M{"label": label}).GetAll(&dbSecrets)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Annotatef(err, "querying secret URI for label %q", label)
		}
		return nil
	}); err != nil {
		return nil, errors.Trace(domain.CoerceError(err))
	}

	if len(dbSecrets) == 0 {
		return nil, fmt.Errorf("secret with label %q not found%w", label, errors.Hide(secreterrors.SecretNotFound))
	}
	return coresecrets.ParseURI(dbSecrets[0].ID)
}

// GetSecretRevision returns the secret revision with the given URI and revision number,
// returning an error satisfying [secreterrors.SecretRevisionNotFound] if the secret revision does not exist.
func (st State) GetSecretRevision(ctx context.Context, uri *coresecrets.URI, revision int) (*coresecrets.SecretRevisionMetadata, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var secretRevisions []*coresecrets.SecretRevisionMetadata
	if err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		secretRevisions, err = st.listSecretRevisions(ctx, tx, uri, &revision)
		return errors.Annotatef(err, "querying secret revision %d for %q", revision, uri.ID)
	}); err != nil {
		return nil, errors.Trace(domain.CoerceError(err))
	}

	if len(secretRevisions) == 0 {
		return nil, fmt.Errorf("secret revision %d for %q not found%w", revision, uri, errors.Hide(secreterrors.SecretRevisionNotFound))
	}
	return secretRevisions[0], nil
}

func (st State) listSecretRevisions(ctx context.Context, tx *sqlair.TX, uri *coresecrets.URI, revision *int) ([]*coresecrets.SecretRevisionMetadata, error) {
	query := `
SELECT (*) AS (&secretRevision.*)
FROM   secret_revision
WHERE  secret_id = $secretRevision.secret_id
`
	want := secretRevision{SecretID: uri.ID}
	if revision != nil {
		query = query + "\nAND revision = $secretRevision.revision"
		want.Revision = *revision
	}

	queryStmt, err := st.Prepare(query, secretRevision{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	var dbSecretRevisions secretRevisions
	err = tx.Query(ctx, queryStmt, want).GetAll(&dbSecretRevisions)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Annotatef(err, "retrieving secret revisions for %q", uri)
	}

	return dbSecretRevisions.toSecretRevisions()
}

// GetSecretValue returns the contents - either data or value reference - of a given secret revision,
// returning an error satisfying [secreterrors.SecretRevisionNotFound] if the secret revision does not exist.
func (st State) GetSecretValue(ctx context.Context, uri *coresecrets.URI, revision int) (coresecrets.SecretData, *coresecrets.ValueRef, error) {
	db, err := st.DB()
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	// We look for either content or a value reference, which ever is present.
	contentQuery := `
SELECT (*) AS (&secretContent.*)
FROM   secret_content sc
JOIN   secret_revision rev ON sc.revision_uuid = rev.uuid  
WHERE  rev.secret_id = $secretRevision.secret_id AND rev.revision = $secretRevision.revision
`

	contentQueryStmt, err := st.Prepare(contentQuery, secretContent{}, secretRevision{})
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	valueRefQuery := `
SELECT (*) AS (&secretValueRef.*)
FROM   secret_value_ref val
JOIN   secret_revision rev ON val.revision_uuid = rev.uuid  
WHERE  rev.secret_id = $secretRevision.secret_id AND rev.revision = $secretRevision.revision
`

	valueRefQueryStmt, err := st.Prepare(valueRefQuery, secretValueRef{}, secretRevision{})
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	want := secretRevision{SecretID: uri.ID, Revision: revision}

	var (
		dbSecretValues    secretValues
		dbSecretValueRefs []secretValueRef
	)
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, contentQueryStmt, want).GetAll(&dbSecretValues)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Annotatef(err, "retrieving secret value for %q revision %d", uri, revision)
		}
		// Do we have content from the db?
		if len(dbSecretValues) > 0 {
			return nil
		}

		// No content, try a value reference.
		err = tx.Query(ctx, valueRefQueryStmt, want).GetAll(&dbSecretValueRefs)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return errors.Annotatef(err, "retrieving secret value ref for %q revision %d", uri, revision)
	}); err != nil {
		return nil, nil, errors.Annotate(err, "querying secret value")
	}

	// Compose and return any secret content from the db.
	if len(dbSecretValues) > 0 {
		content, err := dbSecretValues.toSecretData()
		if err != nil {
			return nil, nil, errors.Annotatef(err, "composing secret content for secret %q revision %d from database", uri, revision)
		}
		return content, nil, nil
	}

	// Process any value reference.
	if len(dbSecretValueRefs) == 0 {
		return nil, nil, fmt.Errorf("secret value ref for %q revision %d not found%w", uri, revision, errors.Hide(secreterrors.SecretRevisionNotFound))
	}
	if len(dbSecretValueRefs) != 1 {
		return nil, nil, fmt.Errorf("unexpected secret value refs for %q revision %d: got %d values", uri, revision, len(dbSecretValues))
	}
	return nil, &coresecrets.ValueRef{
		BackendID:  dbSecretValueRefs[0].BackendUUID,
		RevisionID: dbSecretValueRefs[0].RevisionID,
	}, nil
}

// checkExistsIfLocal returns an error satisfying [secreterrors.SecretNotFound] if the specified
// secret URI is from this model and the secret it refers to does not exist in the model.
func (st State) checkExistsIfLocal(ctx context.Context, tx *sqlair.TX, uri *coresecrets.URI) error {
	query := `
SELECT ok as &M.ok FROM (
    SELECT True as ok FROM secret
    WHERE 
        (EXISTS (SELECT uuid FROM model WHERE uuid = $M.uuid) OR $M.uuid = '')
        AND secret.id = $M.secret_id
    UNION
    SELECT True as ok FROM model
    WHERE
        NOT EXISTS (SELECT uuid FROM model WHERE uuid = $M.uuid)
)
`
	queryStmt, err := st.Prepare(query, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}
	result := sqlair.M{}
	err = tx.Query(ctx, queryStmt, sqlair.M{"secret_id": uri.ID, "uuid": uri.SourceUUID}).Get(&result)
	if err == nil {
		return nil
	}
	if errors.Is(err, sqlair.ErrNoRows) {
		return secreterrors.SecretNotFound
	} else {
		return errors.Annotatef(err, "looking up secret URI %q", uri)
	}
}

// GetSecretConsumer returns the secret consumer info for the specified unit and secret, along with
// the latest revision for the secret.
// If the unit does not exist, an error satisfying [uniterrors.NotFound] is returned.
// If the secret does not exist, an error satisfying [secreterrors.SecretNotFound] is returned.
// If there's not currently a consumer record for the secret, the latest revision is still returned,
// along with an error satisfying [secreterrors.SecretConsumerNotFound].
func (st State) GetSecretConsumer(ctx context.Context, uri *coresecrets.URI, unitName string) (*coresecrets.SecretConsumerMetadata, int, error) {
	db, err := st.DB()
	if err != nil {
		return nil, 0, errors.Trace(err)
	}

	consumer := secretUnitConsumer{
		SecretID: uri.ID,
	}

	query := `
SELECT 
     suc.label AS &secretUnitConsumer.label,
     suc.current_revision AS &secretUnitConsumer.current_revision
FROM secret_unit_consumer suc
WHERE suc.secret_id = $secretUnitConsumer.secret_id
AND   suc.unit_uuid = $secretUnitConsumer.unit_uuid
`

	queryStmt, err := st.Prepare(query, secretUnitConsumer{})
	if err != nil {
		return nil, 0, errors.Trace(err)
	}

	selectUnitUUID := `SELECT &M.uuid FROM unit WHERE unit_id=$M.unit_id`
	selectUnitUUIDStmt, err := st.Prepare(selectUnitUUID, sqlair.M{})
	if err != nil {
		return nil, 0, errors.Trace(err)
	}

	selectLatestRevision := `
SELECT MAX(revision) AS &M.latest_revision
FROM secret_revision rev
WHERE rev.secret_id = $M.secret_id`
	selectLatestRevisionStmt, err := st.Prepare(selectLatestRevision, sqlair.M{})
	if err != nil {
		return nil, 0, errors.Trace(err)
	}

	var (
		dbSecretConsumers secretUnitConsumers
		latestRevision    int
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := st.checkExistsIfLocal(ctx, tx, uri); err != nil {
			return errors.Trace(err)
		}

		result := sqlair.M{}
		err = tx.Query(ctx, selectUnitUUIDStmt, sqlair.M{"unit_id": unitName}).Get(&result)
		if err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return uniterrors.NotFound
			} else {
				return errors.Annotatef(err, "looking up unit UUID for %q", unitName)
			}
		}
		consumer.UnitUUID = result["uuid"].(string)
		err = tx.Query(ctx, queryStmt, consumer).GetAll(&dbSecretConsumers)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Annotate(err, "querying secret consumers")
		}

		// TODO(secrets) - we need something different for cross model secrets
		err = tx.Query(ctx, selectLatestRevisionStmt, sqlair.M{"secret_id": uri.ID}).Get(&result)
		if err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return secreterrors.SecretNotFound
			} else {
				return errors.Annotatef(err, "looking up latest revision for %q", uri.ID)
			}
		}
		rev, _ := result["latest_revision"].(int64)
		latestRevision = int(rev)

		return nil
	})
	if err != nil {
		return nil, 0, errors.Trace(domain.CoerceError(err))
	}
	if len(dbSecretConsumers) == 0 {
		return nil, latestRevision, fmt.Errorf("secret consumer for %q and unit %q%w", uri.ID, unitName, secreterrors.SecretConsumerNotFound)
	}
	consumers, err := dbSecretConsumers.toSecretConsumers()
	if err != nil {
		return nil, 0, errors.Trace(err)
	}
	return consumers[0], latestRevision, nil
}

// SaveSecretConsumer saves the consumer metadata for the given secret and unit.
// If the unit does not exist, an error satisfying [uniterrors.NotFound] is returned.
// If the secret does not exist, an error satisfying [secreterrors.SecretNotFound] is returned.
func (st State) SaveSecretConsumer(ctx context.Context, uri *coresecrets.URI, unitName string, md *coresecrets.SecretConsumerMetadata) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	insertQuery := `
INSERT INTO secret_unit_consumer (*)
VALUES ($secretUnitConsumer.*)
ON CONFLICT(secret_id, unit_uuid) DO UPDATE SET
    label=excluded.label,
    current_revision=excluded.current_revision
`

	insertStmt, err := st.Prepare(insertQuery, secretUnitConsumer{})
	if err != nil {
		return errors.Trace(err)
	}

	selectUnitUUID := `select &M.uuid FROM unit WHERE unit_id=$M.unit_id`
	selectUnitUUIDStmt, err := st.Prepare(selectUnitUUID, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}

	consumer := secretUnitConsumer{
		SecretID:        uri.ID,
		Label:           md.Label,
		CurrentRevision: md.CurrentRevision,
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := st.checkExistsIfLocal(ctx, tx, uri); err != nil {
			return errors.Trace(err)
		}
		result := sqlair.M{}
		err = tx.Query(ctx, selectUnitUUIDStmt, sqlair.M{"unit_id": unitName}).Get(&result)
		if err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return uniterrors.NotFound
			} else {
				return errors.Annotatef(err, "looking up unit UUID for %q", unitName)
			}
		}
		consumer.UnitUUID = result["uuid"].(string)
		if err := tx.Query(ctx, insertStmt, consumer).Run(); err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	return errors.Trace(domain.CoerceError(err))
}
