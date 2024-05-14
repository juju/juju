// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/watcher/eventsource"
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
	logger logger.Logger
}

// NewState returns a new secretMetadata state
// based on the input database factory method.
func NewState(factory coredatabase.TxnRunnerFactory, logger logger.Logger) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
		logger:    logger,
	}
}

// GetModelUUID returns the uuid of the model,
// or an error satisfying [modelerrors.NotFound]
func (st State) GetModelUUID(ctx context.Context) (string, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Trace(err)
	}

	var modelUUID string
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		modelUUID, err = st.getModelUUID(ctx, tx)
		return err
	})
	return modelUUID, errors.Trace(domain.CoerceError(err))
}

func (st State) getModelUUID(ctx context.Context, tx *sqlair.TX) (string, error) {
	getModelUUIDSQL := "SELECT &M.uuid FROM model"
	getModelUUIDStmt, err := st.Prepare(getModelUUIDSQL, sqlair.M{})
	if err != nil {
		return "", errors.Trace(err)
	}

	result := sqlair.M{}
	err = tx.Query(ctx, getModelUUIDStmt).Get(&result)
	if err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return "", modelerrors.NotFound
		} else {
			return "", errors.Annotatef(err, "looking up model UUID")
		}
	}
	return result["uuid"].(string), nil
}

// CreateUserSecret creates a user secret, returning an error satisfying
// [secreterrors.SecretAlreadyExists]if a user secret with the same
// label already exists.
func (st State) CreateUserSecret(
	ctx context.Context, version int, uri *coresecrets.URI, secret domainsecret.UpsertSecretParams,
) error {
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

		label := ""
		if secret.Label != nil {
			label = *secret.Label
		}
		dbSecretOwner := secretModelOwner{SecretID: uri.ID, Label: label}
		if err := st.upsertSecretModelOwner(ctx, tx, dbSecretOwner); err != nil {
			return errors.Annotatef(err, "inserting user secret record for secret %q", uri)
		}

		modelUUID, err := st.getModelUUID(ctx, tx)
		if err != nil {
			return errors.Trace(err)
		}

		if err := st.grantSecretOwnerManage(ctx, tx, uri, modelUUID, domainsecret.SubjectModel); err != nil {
			return errors.Annotatef(err, "granting owner manage access for secret %q", uri)
		}

		return nil
	})
	return errors.Trace(domain.CoerceError(err))
}

// checkSecretUserLabelExists returns an error if a user
// secret with the given label already exists.
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

// CreateCharmApplicationSecret creates a secret onwed by the specified
// application, returning an error satisfying [secreterrors.SecretAlreadyExists]
// if a secretowned by the same application with the same label already exists.
// It also returns an error satisfying [applicationerrors.ApplicationNotFound]
// ifthe application does not exist.
func (st State) CreateCharmApplicationSecret(
	ctx context.Context, version int, uri *coresecrets.URI, appName string, secret domainsecret.UpsertSecretParams,
) error {
	if secret.AutoPrune != nil && *secret.AutoPrune {
		return secreterrors.AutoPruneNotSupported
	}

	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	revisionUUID, err := uuid.NewUUID()
	if err != nil {
		return errors.Trace(err)
	}

	label := ""
	if secret.Label != nil {
		label = *secret.Label
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		dbSecretOwner := secretApplicationOwner{SecretID: uri.ID, Label: label}

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
				return errors.Annotatef(err, "looking up application UUID for %q", appName)
			}
		}
		dbSecretOwner.ApplicationUUID = result["uuid"].(string)

		checkExists := st.checkApplicationSecretLabelExists(dbSecretOwner.ApplicationUUID)
		if err := st.createSecret(ctx, tx, version, uri, secret, revisionUUID, checkExists); err != nil {
			return errors.Annotatef(err, "inserting secret records for secret %q", uri)
		}

		if err := st.upsertSecretApplicationOwner(ctx, tx, dbSecretOwner); err != nil {
			return errors.Annotatef(err, "inserting application secret record for secret %q", uri)
		}

		if err := st.grantSecretOwnerManage(
			ctx, tx, uri, dbSecretOwner.ApplicationUUID, domainsecret.SubjectApplication,
		); err != nil {
			return errors.Annotatef(err, "granting owner manage access for secret %q", uri)
		}
		return nil
	})
	return errors.Trace(domain.CoerceError(err))
}

// checkApplicationSecretLabelExists returns function which checks if
// a charm application secret with the given label already exists.
func (st State) checkApplicationSecretLabelExists(app_uuid string) checkExistsFunc {
	return func(ctx context.Context, tx *sqlair.TX, label string) error {
		if label == "" {
			return nil
		}

		// TODO(secrets) - we check using 2 queries, but should do in DDL
		checkLabelExistsSQL := `
SELECT secret_id AS &secretApplicationOwner.secret_id
FROM (
    SELECT secret_id
    FROM   secret_application_owner
    WHERE  label = $secretApplicationOwner.label
    AND    application_uuid = $secretApplicationOwner.application_uuid
    UNION
    SELECT secret_id
    FROM   secret_unit_owner
           JOIN unit u ON u.uuid = unit_uuid
    WHERE  label = $secretApplicationOwner.label
    AND    u.application_uuid = $secretApplicationOwner.application_uuid
)`

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
			return fmt.Errorf(
				"secret with label %q already exists%w", label, errors.Hide(secreterrors.SecretLabelAlreadyExists))
		}
		return nil
	}
}

// CreateCharmUnitSecret creates a secret onwed by the specified unit,
// returning an error satisfying [secreterrors.SecretAlreadyExists] if a secret
// owned by the same unit with the same label already exists.
// It also returns an error satisfying [uniterrors.NotFound] if
// the unit does not exist.
func (st State) CreateCharmUnitSecret(
	ctx context.Context, version int, uri *coresecrets.URI, unitName string, secret domainsecret.UpsertSecretParams,
) error {
	if secret.AutoPrune != nil && *secret.AutoPrune {
		return secreterrors.AutoPruneNotSupported
	}

	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	revisionUUID, err := uuid.NewUUID()
	if err != nil {
		return errors.Trace(err)
	}

	label := ""
	if secret.Label != nil {
		label = *secret.Label
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		dbSecretOwner := secretUnitOwner{SecretID: uri.ID, Label: label}

		selectUnitUUID := `SELECT &unit.uuid FROM unit WHERE unit_id=$unit.unit_id`
		selectUnitUUIDStmt, err := st.Prepare(selectUnitUUID, unit{})
		if err != nil {
			return errors.Trace(err)
		}

		result := unit{}
		err = tx.Query(ctx, selectUnitUUIDStmt, unit{UnitName: unitName}).Get(&result)
		if err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return fmt.Errorf("unit %q not found%w", unitName, errors.Hide(uniterrors.NotFound))
			} else {
				return errors.Annotatef(err, "looking up unit UUID for %q", unitName)
			}
		}
		dbSecretOwner.UnitUUID = result.UUID

		checkExists := st.checkUnitSecretLabelExists(dbSecretOwner.UnitUUID)
		if err := st.createSecret(ctx, tx, version, uri, secret, revisionUUID, checkExists); err != nil {
			return errors.Annotatef(err, "inserting secret records for secret %q", uri)
		}

		if err := st.upsertSecretUnitOwner(ctx, tx, dbSecretOwner); err != nil {
			return errors.Annotatef(err, "inserting unit secret record for secret %q", uri)
		}

		if err := st.grantSecretOwnerManage(ctx, tx, uri, dbSecretOwner.UnitUUID, domainsecret.SubjectUnit); err != nil {
			return errors.Annotatef(err, "granting owner manage access for secret %q", uri)
		}
		return nil
	})
	return errors.Trace(domain.CoerceError(err))
}

// UpdateSecret creates a secret with the specified parameters, returning an
// errorsatisfying [secreterrors.SecretNotFound] if the secret does not exist.
// It also returns an error satisfying [secreterrors.SecretLabelAlreadyExists]
// if the secret owner already has a secret with the same label.
func (st State) UpdateSecret(ctx context.Context, uri *coresecrets.URI, secret domainsecret.UpsertSecretParams) error {
	if !secret.HasUpdate() {
		return errors.New("must specify a new value or metadata to update a secret")
	}

	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := st.updateSecret(ctx, tx, uri, secret); err != nil {
			return errors.Annotatef(err, "updating secret records for secret %q", uri)
		}
		return nil
	})
	return errors.Trace(domain.CoerceError(err))
}

// checkUnitSecretLabelExists returns function which checks if a
// charm unit secret with the given label already exists.
func (st State) checkUnitSecretLabelExists(unit_uuid string) checkExistsFunc {
	return func(ctx context.Context, tx *sqlair.TX, label string) error {
		if label == "" {
			return nil
		}

		// TODO(secrets) - we check using 2 queries, but should do in DDL
		checkLabelExistsSQL := `
SELECT secret_id AS &secretUnitOwner.secret_id
FROM (
    SELECT secret_id
    FROM   secret_application_owner sao
           JOIN unit u ON sao.application_uuid = u.application_uuid
    WHERE  label = $secretUnitOwner.label
    AND    u.uuid = $secretUnitOwner.unit_uuid
    UNION
    SELECT DISTINCT secret_id
    FROM   secret_unit_owner suo
           JOIN unit u ON suo.unit_uuid = u.uuid
           JOIN unit peer ON peer.application_uuid = u.application_uuid
    WHERE  label = $secretUnitOwner.label
    AND peer.uuid != u.uuid
)`

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
			return fmt.Errorf(
				"secret with label %q already exists%w", label, errors.Hide(secreterrors.SecretLabelAlreadyExists))
		}
		return nil
	}
}

type checkExistsFunc = func(ctx context.Context, tx *sqlair.TX, label string) error

// createSecret creates the records needed to store secret data,
// excluding secret owner records.
func (st State) createSecret(
	ctx context.Context, tx *sqlair.TX, version int, uri *coresecrets.URI,
	secret domainsecret.UpsertSecretParams, revisionUUID uuid.UUID,
	checkExists checkExistsFunc,
) error {
	if len(secret.Data) == 0 && secret.ValueRef == nil {
		return errors.Errorf("cannot create a secret without content")
	}
	if secret.Label != nil && *secret.Label != "" {
		if err := checkExists(ctx, tx, *secret.Label); err != nil {
			return errors.Trace(err)
		}
	}

	insertQuery := `
INSERT INTO secret (id)
VALUES ($secretID.id)`

	insertStmt, err := st.Prepare(insertQuery, secretID{})
	if err != nil {
		return errors.Trace(err)
	}

	err = tx.Query(ctx, insertStmt, secretID{ID: uri.ID}).Run()
	if err != nil {
		return errors.Trace(err)
	}

	now := time.Now().UTC()
	dbSecret := secretMetadata{
		ID:         uri.ID,
		Version:    version,
		CreateTime: now,
		UpdateTime: now,
	}
	updateSecretMetadataFromParams(secret, &dbSecret)
	if err := st.upsertSecret(ctx, tx, dbSecret); err != nil {
		return errors.Annotatef(err, "creating user secret %q", uri)
	}

	dbRevision := secretRevision{
		ID:         revisionUUID.String(),
		SecretID:   uri.ID,
		Revision:   1,
		CreateTime: now,
	}

	if err := st.upsertSecretRevision(ctx, tx, dbRevision, secret.ExpireTime); err != nil {
		return errors.Annotatef(err, "inserting revision for secret %q", uri)
	}

	if secret.NextRotateTime != nil {
		if err := st.upsertSecretNextRotateTime(ctx, tx, uri, *secret.NextRotateTime); err != nil {
			return errors.Annotatef(err, "inserting next rotate time for secret %q", uri)
		}
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

// createSecret creates the records needed to store secret data,
// excluding secret owner records.
func (st State) updateSecret(
	ctx context.Context, tx *sqlair.TX, uri *coresecrets.URI,
	secret domainsecret.UpsertSecretParams,
) error {
	// We need the latest revision so far, plus owner info for the secret,
	// so we may as well also include existing metadata as well so simplify
	// the update statement needed.
	existingSecretQuery := `
WITH rev AS (
    SELECT MAX(revision) AS latest_revision
    FROM   secret_revision
    WHERE  secret_id = $secretID.id
)
SELECT (sm.secret_id,
       version,
	   description,
       auto_prune,
       rp.policy,
       rev.latest_revision) AS (&secretInfo.*),
       (so.owner_kind,
       so.owner_id,
       so.label) AS (&secretOwner.*)
FROM   secret_metadata sm, rev
       LEFT JOIN secret_rotate_policy rp ON rp.id = sm.rotate_policy_id
       LEFT JOIN (
          SELECT $ownerKind.model_owner_kind AS owner_kind, '' AS owner_id, label, secret_id
          FROM   secret_model_owner so
          UNION
          SELECT $ownerKind.application_owner_kind AS owner_kind, application.uuid AS owner_id, label, secret_id
          FROM   secret_application_owner so
          JOIN   application
          WHERE  application.uuid = so.application_uuid
          UNION
          SELECT $ownerKind.unit_owner_kind AS owner_kind, unit_uuid AS owner_id, label, secret_id
          FROM   secret_unit_owner so
          JOIN   unit
          WHERE  unit.uuid = so.unit_uuid
       ) so ON so.secret_id = sm.secret_id
WHERE  sm.secret_id = $secretID.id`

	existingSecretStmt, err := st.Prepare(existingSecretQuery, secretID{}, secretInfo{}, secretOwner{}, ownerKindParam)
	if err != nil {
		return errors.Trace(err)
	}

	var (
		dbSecrets      secrets
		dbsecretOwners []secretOwner
	)
	secretIDParam := secretID{ID: uri.ID}
	err = tx.Query(ctx, existingSecretStmt, secretIDParam, ownerKindParam).GetAll(&dbSecrets, &dbsecretOwners)
	if errors.Is(err, sqlair.ErrNoRows) {
		return fmt.Errorf("secret %q not found%w", uri, errors.Hide(secreterrors.SecretNotFound))
	}
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Trace(err)
	}

	existing, err := dbSecrets.toSecretMetadata(dbsecretOwners)
	if err != nil {
		return errors.Trace(err)
	}

	// Check to be sure a duplicate label won't be used.
	var checkExists checkExistsFunc
	switch kind := existing[0].Owner.Kind; kind {
	case coresecrets.ModelOwner:
		checkExists = st.checkUserSecretLabelExists
	case coresecrets.ApplicationOwner:
		if secret.AutoPrune != nil && *secret.AutoPrune {
			return secreterrors.AutoPruneNotSupported
		}
		// Query selects the app uuid as owner id.
		checkExists = st.checkApplicationSecretLabelExists(existing[0].Owner.ID)
	case coresecrets.UnitOwner:
		if secret.AutoPrune != nil && *secret.AutoPrune {
			return secreterrors.AutoPruneNotSupported
		}
		// Query selects the unit uuid as owner id.
		checkExists = st.checkUnitSecretLabelExists(existing[0].Owner.ID)
	default:
		// Should never happen.
		return errors.Errorf("unexpected secret owner kind %q", kind)
	}

	if secret.Label != nil && *secret.Label != "" {
		if err := checkExists(ctx, tx, *secret.Label); err != nil {
			return errors.Trace(err)
		}
	}

	now := time.Now().UTC()
	dbSecret := secretMetadata{
		ID:             dbSecrets[0].ID,
		Version:        dbSecrets[0].Version,
		Description:    dbSecrets[0].Description,
		AutoPrune:      dbSecrets[0].AutoPrune,
		RotatePolicyID: int(domainsecret.MarshallRotatePolicy(&existing[0].RotatePolicy)),
		UpdateTime:     now,
	}
	dbSecret.UpdateTime = now
	updateSecretMetadataFromParams(secret, &dbSecret)
	if err := st.upsertSecret(ctx, tx, dbSecret); err != nil {
		return errors.Annotatef(err, "updating secret %q", uri)
	}

	if secret.Label != nil {
		if err := st.upsertSecretLabel(ctx, tx, existing[0].URI, *secret.Label, existing[0].Owner); err != nil {
			return errors.Annotatef(err, "updating label for secret %q", uri)
		}
	}

	// Will secret rotate? If not, delete next rotation row.
	if secret.RotatePolicy != nil && *secret.RotatePolicy == domainsecret.RotateNever {
		deleteNextRotate := "DELETE FROM secret_rotation WHERE secret_id=$secretID.id"
		deleteNextRotateStmt, err := st.Prepare(deleteNextRotate, secretID{})
		if err != nil {
			return errors.Trace(err)
		}
		err = tx.Query(ctx, deleteNextRotateStmt, secretIDParam).Run()
		if err != nil {
			return errors.Annotatef(err, "deleting next rotate record for secret %q", uri)
		}
	}

	if len(secret.Data) == 0 && secret.ValueRef == nil {
		return nil
	}

	revisionUUID, err := uuid.NewUUID()
	if err != nil {
		return errors.Trace(err)
	}

	nextRevision := existing[0].LatestRevision + 1
	dbRevision := secretRevision{
		ID:         revisionUUID.String(),
		SecretID:   uri.ID,
		Revision:   nextRevision,
		CreateTime: now,
	}

	if err := st.upsertSecretRevision(ctx, tx, dbRevision, secret.ExpireTime); err != nil {
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

	if err := st.markObsoleteRevisions(ctx, tx, uri); err != nil {
		return errors.Annotatef(err, "marking obsolete revisions for secret %q", uri)
	}

	if dbSecret.AutoPrune {
		if err := st.pruneUnusedRevisions(ctx, tx, uri); err != nil {
			return errors.Annotatef(err, "deleting unused revisions for secret %q", uri)
		}
	}
	return nil
}

// markObsoleteRevisions obsoletes the revisions and sets the pending_delete
// to true in the secret_revision table for the specified secret if the
// revision is not the latest revision and there are no consumers for the
// revision.
func (st State) markObsoleteRevisions(ctx context.Context, tx *sqlair.TX, uri *coresecrets.URI) error {
	query, err := st.Prepare(`
WITH in_use AS (
    -- revisions that have local consumers.
    SELECT DISTINCT current_revision FROM secret_unit_consumer suc
    WHERE  suc.secret_id = $secretRef.secret_id
    UNION
    -- revisions that have remote consumers.
    SELECT DISTINCT current_revision FROM secret_remote_unit_consumer suc
    WHERE  suc.secret_id = $secretRef.secret_id
    UNION
    -- the latest revision.
    SELECT MAX(revision) FROM secret_revision rev
    WHERE  rev.secret_id = $secretRef.secret_id
)
SELECT sr.uuid AS &secretRevision.uuid
FROM   secret_revision sr
WHERE  sr.secret_id = $secretRef.secret_id
AND    sr.revision NOT IN (SELECT * FROM in_use)`, secretRef{}, secretRevision{})
	if err != nil {
		return errors.Trace(err)
	}

	stmt, err := st.Prepare(`
INSERT INTO secret_revision_obsolete (*)
VALUES ($secretRevisionObsolete.*)
ON CONFLICT(revision_uuid) DO UPDATE SET
    obsolete=excluded.obsolete,
    pending_delete=excluded.pending_delete`, secretRevisionObsolete{})
	if err != nil {
		return errors.Trace(err)
	}

	var revisionUUIIDs secretRevisions
	err = tx.Query(ctx, query, secretRef{ID: uri.ID}).GetAll(&revisionUUIIDs)
	if errors.Is(err, sqlair.ErrNoRows) {
		// No obsolete revisions to mark.
		return nil
	}
	if err != nil {
		return errors.Trace(err)
	}

	for _, revisionUUID := range revisionUUIIDs {
		// TODO: use bulk insert.
		obsolete := secretRevisionObsolete{
			ID:            revisionUUID.ID,
			Obsolete:      true,
			PendingDelete: true,
		}
		err = tx.Query(ctx, stmt, obsolete).Run()
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (st State) upsertSecretLabel(
	ctx context.Context, tx *sqlair.TX, uri *coresecrets.URI, label string, owner coresecrets.Owner,
) error {
	switch owner.Kind {
	case coresecrets.ModelOwner:
		dbSecretOwner := secretModelOwner{
			SecretID: uri.ID,
			Label:    label,
		}
		if err := st.upsertSecretModelOwner(ctx, tx, dbSecretOwner); err != nil {
			return errors.Annotatef(err, "updating model secret record for secret %q", uri)
		}
	case coresecrets.ApplicationOwner:
		dbSecretOwner := secretApplicationOwner{
			SecretID: uri.ID,
			// Query selects the application uuid as owner id.
			ApplicationUUID: owner.ID,
			Label:           label,
		}
		if err := st.upsertSecretApplicationOwner(ctx, tx, dbSecretOwner); err != nil {
			return errors.Annotatef(err, "updating application secret record for secret %q", uri)
		}
	case coresecrets.UnitOwner:
		dbSecretOwner := secretUnitOwner{
			SecretID: uri.ID,
			// Query selects the unit uuid as owner id.
			UnitUUID: owner.ID,
			Label:    label,
		}
		if err := st.upsertSecretUnitOwner(ctx, tx, dbSecretOwner); err != nil {
			return errors.Annotatef(err, "updating unit secret record for secret %q", uri)
		}
	}
	return nil
}

func (st State) pruneUnusedRevisions(ctx context.Context, tx *sqlair.TX, uri *coresecrets.URI) error {
	// TODO(secrets)
	return nil
	//var md *secrets.SecretMetadata
	//if !md.AutoPrune {
	//	return md, nil
	//}
	//// If the secret was updated, we need to delete the old unused secret revisions.
	//revsToDelete, err := s.ListUnusedSecretRevisions(ctx, uri)
	//if err != nil {
	//	return nil, errors.Trace(err)
	//}
	//var revisions []int
	//for _, rev := range revsToDelete {
	//	if rev == md.LatestRevision {
	//		// We don't want to delete the latest revision.
	//		continue
	//	}
	//	revisions = append(revisions, rev)
	//}
	//if len(revisions) == 0 {
	//	return md, nil
	//}
	//err = s.DeleteUserSecret(ctx, uri, revisions, func(uri *secrets.URI) error { return nil })
	//if err != nil {
	//	// We don't want to fail the update if we can't prune the unused secret revisions because they will be picked up later
	//	// when the secret has any new obsolete revisions.
	//	s.logger.Warningf("failed to prune unused secret revisions for %q: %v", uri, err)
	//}
	//return md, nil
}

func updateSecretMetadataFromParams(p domainsecret.UpsertSecretParams, md *secretMetadata) {
	if p.Description != nil {
		md.Description = *p.Description
	}
	if p.AutoPrune != nil {
		md.AutoPrune = *p.AutoPrune
	}
	if p.RotatePolicy != nil {
		md.RotatePolicyID = int(*p.RotatePolicy)
	}
}

func (st State) upsertSecret(ctx context.Context, tx *sqlair.TX, dbSecret secretMetadata) error {
	insertMetadataQuery := `
INSERT INTO secret_metadata (*)
VALUES ($secretMetadata.*)
ON CONFLICT(secret_id) DO UPDATE SET
    version=excluded.version,
    description=excluded.description,
    rotate_policy_id=excluded.rotate_policy_id,
    auto_prune=excluded.auto_prune,
    update_time=excluded.update_time
`

	insertMetadataStmt, err := st.Prepare(insertMetadataQuery, secretMetadata{})
	if err != nil {
		return errors.Trace(err)
	}

	err = tx.Query(ctx, insertMetadataStmt, dbSecret).Run()
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (st State) grantSecretOwnerManage(
	ctx context.Context, tx *sqlair.TX, uri *coresecrets.URI, ownerUUID string, ownerType domainsecret.GrantSubjectType,
) error {
	perm := secretPermission{
		SecretID:      uri.ID,
		RoleID:        domainsecret.RoleManage,
		SubjectUUID:   ownerUUID,
		SubjectTypeID: ownerType,
		ScopeUUID:     ownerUUID,
	}
	switch ownerType {
	case domainsecret.SubjectUnit:
		perm.ScopeTypeID = domainsecret.ScopeUnit
	case domainsecret.SubjectApplication:
		perm.ScopeTypeID = domainsecret.ScopeApplication
	case domainsecret.SubjectModel:
		perm.ScopeTypeID = domainsecret.ScopeModel
	}
	return st.grantAccess(ctx, tx, perm)
}

func (st State) upsertSecretModelOwner(ctx context.Context, tx *sqlair.TX, owner secretModelOwner) error {
	insertQuery := `
INSERT INTO secret_model_owner (secret_id, label)
VALUES      ($secretModelOwner.*)
ON CONFLICT(secret_id) DO UPDATE SET label=excluded.label`

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
ON CONFLICT(secret_id, application_uuid) DO UPDATE SET label=excluded.label`

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
VALUES ($secretUnitOwner.*)
ON CONFLICT(secret_id, unit_uuid) DO UPDATE SET label=excluded.label`

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

func (st State) upsertSecretNextRotateTime(
	ctx context.Context, tx *sqlair.TX, uri *coresecrets.URI, next time.Time,
) error {
	insertQuery := `
INSERT INTO secret_rotation (*)
VALUES ($secretRotate.*)
ON CONFLICT(secret_id) DO UPDATE SET
    next_rotation_time=excluded.next_rotation_time`

	rotate := secretRotate{SecretID: uri.ID, NextRotateTime: next.UTC()}
	insertStmt, err := st.Prepare(insertQuery, rotate)
	if err != nil {
		return errors.Trace(err)
	}

	err = tx.Query(ctx, insertStmt, rotate).Run()
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (st State) upsertSecretRevision(
	ctx context.Context, tx *sqlair.TX, dbRevision secretRevision, expireTime *time.Time,
) error {
	insertQuery := `
INSERT INTO secret_revision (*)
VALUES ($secretRevision.*)`

	insertStmt, err := st.Prepare(insertQuery, secretRevision{})
	if err != nil {
		return errors.Trace(err)
	}

	err = tx.Query(ctx, insertStmt, dbRevision).Run()
	if err != nil || expireTime == nil {
		return errors.Trace(err)
	}

	insertExpireTimeQuery := `
INSERT INTO secret_revision_expire (*)
VALUES ($secretRevisionExpire.*)
ON CONFLICT(revision_uuid) DO UPDATE SET
    expire_time=excluded.expire_time`

	expire := secretRevisionExpire{RevisionUUID: dbRevision.ID, ExpireTime: expireTime.UTC()}
	insertExpireTimeStmt, err := st.Prepare(insertExpireTimeQuery, expire)
	if err != nil {
		return errors.Trace(err)
	}

	err = tx.Query(ctx, insertExpireTimeStmt, expire).Run()
	if err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (st State) upsertSecretValueRef(
	ctx context.Context, tx *sqlair.TX, revisionUUID string, valueRef *coresecrets.ValueRef,
) error {
	insertQuery := `
INSERT INTO secret_value_ref (*)
VALUES ($secretValueRef.*)
ON CONFLICT(revision_uuid) DO UPDATE SET
    backend_uuid=excluded.backend_uuid,
    revision_id=excluded.revision_id`

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

func (st State) updateSecretContent(
	ctx context.Context, tx *sqlair.TX, revUUID string, content coresecrets.SecretData,
) error {
	// Delete any keys no longer in the content map.
	deleteQuery := `
DELETE FROM secret_content
WHERE  revision_uuid = $revisionUUID.uuid
AND    name NOT IN ($keysToKeep[:])`

	deleteStmt, err := st.Prepare(deleteQuery, revisionUUID{}, keysToKeep{})
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
    content=excluded.content`

	insertStmt, err := st.Prepare(insertQuery, secretContent{})
	if err != nil {
		return errors.Trace(err)
	}

	var keys keysToKeep
	for k := range content {
		keys = append(keys, k)
	}
	if err := tx.Query(ctx, deleteStmt, revisionUUID{UUID: revUUID}, keys).Run(); err != nil {
		return errors.Trace(err)
	}
	for key, value := range content {
		if err := tx.Query(ctx, insertStmt, secretContent{
			RevisionUUID: revUUID,
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

	var revisionNotFoundErr error
	if revision != nil {
		revisionNotFoundErr = fmt.Errorf(
			"secret revision %d for %s not found%w", *revision, uri, errors.Hide(secreterrors.SecretRevisionNotFound))
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
			if revision != nil && len(secretRevisions) == 0 {
				return revisionNotFoundErr
			}
		}
		return nil
	}); err != nil {
		return nil, nil, errors.Trace(domain.CoerceError(err))
	}
	if revision != nil && len(secrets) == 0 {
		return nil, nil, revisionNotFoundErr
	}

	return secrets, revisionResult, nil
}

// GetSecret returns the secret with the given URI, returning an error satisfying [secreterrors.SecretNotFound]
// if the secret does not exist.
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

	query := `
WITH rev AS (
    SELECT   secret_id, MAX(revision) AS latest_revision
    FROM     secret_revision
    GROUP BY secret_id,
),
exp AS (
    SELECT   secret_id, expire_time AS latest_expire_time
    FROM     secret_revision sr
    JOIN     secret_revision_expire sre ON  sre.revision_uuid = sr.uuid
    GROUP BY secret_id
)
SELECT (sm.secret_id,
       version,
       description,
       auto_prune,
       create_time,
       update_time,
       rp.policy,
       sr.next_rotation_time,
       exp.latest_expire_time,
       rev.latest_revision) AS (&secretInfo.*),
       (so.owner_kind,
       so.owner_id,
       so.label) AS (&secretOwner.*)
FROM   secret_metadata sm
       JOIN rev ON rev.secret_id = sm.secret_id
       LEFT JOIN exp ON exp.secret_id = sm.secret_id
       LEFT JOIN secret_rotate_policy rp ON rp.id = sm.rotate_policy_id
       LEFT JOIN secret_rotation sr ON sr.secret_id = sm.secret_id
       LEFT JOIN (
          SELECT $ownerKind.model_owner_kind AS owner_kind, (SELECT uuid FROM model) AS owner_id, label, secret_id
          FROM   secret_model_owner so
          UNION
          SELECT $ownerKind.application_owner_kind AS owner_kind, application.name AS owner_id, label, secret_id
          FROM   secret_application_owner so
          JOIN   application
          WHERE  application.uuid = so.application_uuid
          UNION
          SELECT $ownerKind.unit_owner_kind AS owner_kind, unit.unit_id AS owner_id, label, secret_id
          FROM   secret_unit_owner so
          JOIN   unit
          WHERE  unit.uuid = so.unit_uuid
       ) so ON so.secret_id = sm.secret_id`

	queryTypes := []any{
		secretInfo{},
		secretOwner{},
		ownerKindParam,
	}
	queryParams := []any{ownerKindParam}
	if uri != nil {
		queryTypes = append(queryTypes, secretID{})
		query = query + "\nWHERE sm.secret_id = $secretID.id"
		queryParams = append(queryParams, secretID{ID: uri.ID})
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
    GROUP BY secret_id),
exp AS
    (SELECT  secret_id, expire_time AS latest_expire_time
    FROM     secret_revision sr
    JOIN     secret_revision_expire sre ON  sre.revision_uuid = sr.uuid
    GROUP BY secret_id)`[1:]}

	appOwnerSelect := `
app_owners AS
    (SELECT $ownerKind.application_owner_kind AS owner_kind, application.name AS owner_id, label, secret_id
     FROM   secret_application_owner so
     JOIN   application ON application.uuid = so.application_uuid
     AND application.name IN ($ApplicationOwners[:]))`[1:]

	unitOwnerSelect := `
unit_owners AS
    (SELECT $ownerKind.unit_owner_kind AS owner_kind, unit.unit_id AS owner_id, label, secret_id
     FROM   secret_unit_owner so
     JOIN   unit ON unit.uuid = so.unit_uuid
     AND unit.unit_id IN ($UnitOwners[:]))`[1:]

	if len(appOwners) > 0 {
		preQueryParts = append(preQueryParts, appOwnerSelect)
	}
	if len(unitOwners) > 0 {
		preQueryParts = append(preQueryParts, unitOwnerSelect)
	}
	queryParts := []string{strings.Join(preQueryParts, ",\n")}

	query := `
SELECT (sm.secret_id,
       version,
       description,
       auto_prune,
       rp.policy,
       sr.next_rotation_time,
       exp.latest_expire_time,
       create_time,
       update_time,
       rev.latest_revision) AS (&secretInfo.*),
       (so.owner_kind,
       so.owner_id,
       so.label) AS (&secretOwner.*)
FROM   secret_metadata sm
       JOIN rev ON rev.secret_id = sm.secret_id
       LEFT JOIN exp ON exp.secret_id = sm.secret_id
       LEFT JOIN secret_rotate_policy rp ON rp.id = sm.rotate_policy_id
       LEFT JOIN secret_rotation sr ON sr.secret_id = sm.secret_id`

	queryParts = append(queryParts, query)

	queryTypes := []any{
		secretInfo{},
		secretOwner{},
		ownerKindParam,
	}

	queryParams := []any{ownerKindParam}
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
    ) so ON so.secret_id = sm.secret_id
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

// ListUserSecretsToDrain returns secret drain revision info for any user secrets.
func (st State) ListUserSecretsToDrain(ctx context.Context) ([]*coresecrets.SecretMetadataForDrain, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	query := `
SELECT sm.secret_id AS &secretID.id,
       svr.backend_uuid AS &secretExternalRevision.backend_uuid,
       svr.revision_id AS &secretExternalRevision.revision_id,
       rev.revision AS &secretExternalRevision.revision
FROM   secret_metadata sm
       JOIN secret_revision rev ON rev.secret_id = sm.secret_id
       LEFT JOIN secret_value_ref svr ON svr.revision_uuid = rev.uuid
       JOIN secret_model_owner mso ON mso.secret_id = sm.secret_id`

	queryStmt, err := st.Prepare(query, secretID{}, secretExternalRevision{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	var (
		dbSecrets    secretIDs
		dbsecretRevs secretExternalRevisions
	)

	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, queryStmt).GetAll(&dbSecrets, &dbsecretRevs)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Trace(err)
		}
		return nil
	}); err != nil {
		return nil, errors.Trace(domain.CoerceError(err))
	}

	return dbSecrets.toSecretMetadataForDrain(dbsecretRevs)
}

// ListCharmSecretsToDrain returns secret drain revision info for
// the secrets owned by the specified apps and units.
func (st State) ListCharmSecretsToDrain(
	ctx context.Context,
	appOwners domainsecret.ApplicationOwners, unitOwners domainsecret.UnitOwners,
) ([]*coresecrets.SecretMetadataForDrain, error) {
	if len(appOwners) == 0 && len(unitOwners) == 0 {
		return nil, errors.New("must supply at least one app owner or unit owner")
	}

	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	appOwnedSelect := `
app_owned AS
    (SELECT secret_id
     FROM   secret_application_owner so
     JOIN   application ON application.uuid = so.application_uuid
     AND application.name IN ($ApplicationOwners[:]))`[1:]

	unitOwnedSelect := `
unit_owned AS
    (SELECT secret_id
     FROM   secret_unit_owner so
     JOIN   unit ON unit.uuid = so.unit_uuid
     AND unit.unit_id IN ($UnitOwners[:]))`[1:]

	queryTypes := []any{
		secretID{},
		secretExternalRevision{},
	}

	var (
		preQueryParts []string
		ownerParts    []string
		queryParams   []any
	)
	if len(appOwners) > 0 {
		preQueryParts = append(preQueryParts, appOwnedSelect)
		ownerParts = append(ownerParts, "SELECT secret_id FROM app_owned")
		queryParams = append(queryParams, appOwners)
	}
	if len(unitOwners) > 0 {
		preQueryParts = append(preQueryParts, unitOwnedSelect)
		ownerParts = append(ownerParts, "SELECT secret_id FROM unit_owned")
		queryParams = append(queryParams, unitOwners)
	}
	queryParts := []string{strings.Join(preQueryParts, ",\n")}

	query := `
SELECT
     sm.secret_id AS &secretID.id,
     svr.backend_uuid AS &secretExternalRevision.backend_uuid,
     svr.revision_id AS &secretExternalRevision.revision_id,
     rev.revision AS &secretExternalRevision.revision
FROM secret_metadata sm
JOIN secret_revision rev ON rev.secret_id = sm.secret_id
LEFT JOIN secret_value_ref svr ON svr.revision_uuid = rev.uuid`[1:]

	queryParts = append(queryParts, query)

	ownerJoin := fmt.Sprintf(`
JOIN (
%s
) so ON so.secret_id = sm.secret_id
`[1:], strings.Join(ownerParts, "\nUNION\n"))

	queryParts = append(queryParts, ownerJoin)

	queryStmt, err := st.Prepare("WITH "+strings.Join(queryParts, "\n"), append(queryTypes, queryParams...)...)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var (
		dbSecrets    secretIDs
		dbsecretRevs secretExternalRevisions
	)

	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, queryStmt, queryParams...).GetAll(&dbSecrets, &dbsecretRevs)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Trace(err)
		}
		return nil
	}); err != nil {
		return nil, errors.Trace(domain.CoerceError(err))
	}

	return dbSecrets.toSecretMetadataForDrain(dbsecretRevs)
}

// GetUserSecretURIByLabel returns the URI for the user secret with the specified label,
// or an error satisfying [secreterrors.SecretNotFound] if there's no corresponding URI.
func (st State) GetUserSecretURIByLabel(ctx context.Context, label string) (*coresecrets.URI, error) {
	if label == "" {
		return nil, errors.NotValidf("empty secret label")
	}

	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	query := `
SELECT sm.secret_id AS &secretInfo.secret_id
FROM   secret_metadata sm
JOIN   secret_model_owner mso ON sm.secret_id = mso.secret_id
WHERE  mso.label = $M.label
	`

	queryStmt, err := st.Prepare(query, secretInfo{}, sqlair.M{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	var dbSecrets secrets
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, queryStmt, sqlair.M{"label": label}).GetAll(&dbSecrets)
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

// GetURIByConsumerLabel looks up the secret URI using the label previously
// registered by the specified unit,returning an error satisfying
// [secreterrors.SecretNotFound] if there's no corresponding URI.
// If the unit does not exist, an error satisfying [uniterrors.NotFound]
// is returned.
func (st State) GetURIByConsumerLabel(ctx context.Context, label string, unitName string) (*coresecrets.URI, error) {
	if label == "" {
		return nil, errors.NotValidf("empty secret label")
	}

	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	query := `
SELECT secret_id AS &secretUnitConsumer.secret_id,
       source_model_uuid AS &secretUnitConsumer.source_model_uuid
FROM   secret_unit_consumer suc
WHERE  suc.label = $secretUnitConsumer.label
AND    suc.unit_uuid = $secretUnitConsumer.unit_uuid
`

	queryStmt, err := st.Prepare(query, secretUnitConsumer{}, sqlair.M{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	selectUnitUUID := `select &unit.uuid FROM unit WHERE unit_id=$unit.unit_id`
	selectUnitUUIDStmt, err := st.Prepare(selectUnitUUID, unit{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	var dbConsumers []secretUnitConsumer
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		result := unit{}
		err = tx.Query(ctx, selectUnitUUIDStmt, unit{UnitName: unitName}).Get(&result)
		if err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return fmt.Errorf("unit %q not found%w", unitName, errors.Hide(uniterrors.NotFound))
			} else {
				return errors.Annotatef(err, "looking up unit UUID for %q", unitName)
			}
		}

		suc := secretUnitConsumer{UnitUUID: result.UUID, Label: label}
		err := tx.Query(ctx, queryStmt, suc).GetAll(&dbConsumers)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Annotatef(err, "querying secret URI for label %q", label)
		}
		return nil
	}); err != nil {
		return nil, errors.Trace(domain.CoerceError(err))
	}

	if len(dbConsumers) == 0 {
		return nil, fmt.Errorf(
			"secret with label %q for unit %q not found%w", label, unitName, errors.Hide(secreterrors.SecretNotFound))
	}
	uri, err := coresecrets.ParseURI(dbConsumers[0].SecretID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return uri.WithSource(dbConsumers[0].SourceModelUUID), nil
}

// ListExternalSecretRevisions returns the secret revisions which are stored
// externally ina secret backend, returning an error satisfying
// [secreterrors.SecretNotFound] if the secret does not exist.
func (st State) ListExternalSecretRevisions(
	ctx context.Context, uri *coresecrets.URI, revs ...int) ([]coresecrets.ValueRef, error,
) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	selectRevisionParams := []any{secretID{
		ID: uri.ID,
	}}

	var revFilter string
	if len(revs) > 0 {
		revFilter = "\nAND revision IN ($revisions[:])"
		selectRevisionParams = append(selectRevisionParams, revisions(revs))
	}

	query := fmt.Sprintf(`
SELECT (svr.*) AS (&secretValueRef.*)
FROM   secret_revision sr
JOIN   secret_value_ref svr ON svr.revision_uuid = sr.uuid
WHERE  secret_id = $secretID.id%s
`, revFilter)

	queryStmt, err := st.Prepare(query, append([]any{secretValueRef{}}, selectRevisionParams...)...)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var dbSecretRevisions secretValueRefs
	if err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if isLocal, err := st.checkExistsIfLocal(ctx, tx, uri); err != nil {
			return errors.Trace(err)
		} else if !isLocal {
			// Should never happen.
			return secreterrors.SecretIsNotLocal
		}

		err = tx.Query(ctx, queryStmt, selectRevisionParams...).GetAll(&dbSecretRevisions)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Annotatef(err, "retrieving extrnal secret revisions for %q", uri)
		}
		return nil
	}); err != nil {
		return nil, errors.Trace(domain.CoerceError(err))
	}
	return dbSecretRevisions.toValueRefs(), nil
}

func (st State) listSecretRevisions(
	ctx context.Context, tx *sqlair.TX, uri *coresecrets.URI, revision *int,
) ([]*coresecrets.SecretRevisionMetadata, error) {
	query := `
SELECT (sr.*) AS (&secretRevision.*),
       (svr.*) AS (&secretValueRef.*),
       (sre.*) AS (&secretRevisionExpire.*)
FROM   secret_revision sr
       LEFT JOIN secret_revision_obsolete sro ON sro.revision_uuid = sr.uuid
       LEFT JOIN secret_revision_expire sre ON sre.revision_uuid = sr.uuid
       LEFT JOIN secret_value_ref svr ON svr.revision_uuid = sr.uuid
WHERE  secret_id = $secretRevision.secret_id
`
	want := secretRevision{SecretID: uri.ID}
	if revision != nil {
		query = query + "\nAND revision = $secretRevision.revision"
		want.Revision = *revision
	}

	queryStmt, err := st.Prepare(query, secretRevision{}, secretRevisionObsolete{}, secretRevisionExpire{}, secretValueRef{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	var (
		dbSecretRevisions       secretRevisions
		dbSecretValueRefs       secretValueRefs
		dbSecretRevisionsExpire secretRevisionsExpire
	)
	err = tx.Query(ctx, queryStmt, want).GetAll(&dbSecretRevisions, &dbSecretValueRefs, &dbSecretRevisionsExpire)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Annotatef(err, "retrieving secret revisions for %q", uri)
	}

	return dbSecretRevisions.toSecretRevisions(dbSecretValueRefs, dbSecretRevisionsExpire)
}

// GetSecretValue returns the contents - either data or value reference - of a
// given secret revision, returning an error satisfying
// [secreterrors.SecretRevisionNotFound] if the secret revision does not exist.
func (st State) GetSecretValue(
	ctx context.Context, uri *coresecrets.URI, revision int) (coresecrets.SecretData, *coresecrets.ValueRef, error,
) {
	db, err := st.DB()
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	// We look for either content or a value reference, which ever is present.
	contentQuery := `
SELECT (*) AS (&secretContent.*)
FROM   secret_content sc
       JOIN secret_revision rev ON sc.revision_uuid = rev.uuid
WHERE  rev.secret_id = $secretRevision.secret_id 
AND    rev.revision = $secretRevision.revision`

	contentQueryStmt, err := st.Prepare(contentQuery, secretContent{}, secretRevision{})
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	valueRefQuery := `
SELECT (*) AS (&secretValueRef.*)
FROM   secret_value_ref val
       JOIN secret_revision rev ON val.revision_uuid = rev.uuid
WHERE  rev.secret_id = $secretRevision.secret_id
AND    rev.revision = $secretRevision.revision`

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
		content := dbSecretValues.toSecretData()
		return content, nil, nil
	}

	// Process any value reference.
	if len(dbSecretValueRefs) == 0 {
		return nil, nil, fmt.Errorf(
			"secret value ref for %q revision %d not found%w", uri, revision, errors.Hide(secreterrors.SecretRevisionNotFound))
	}
	if len(dbSecretValueRefs) != 1 {
		return nil, nil, fmt.Errorf(
			"unexpected secret value refs for %q revision %d: got %d values", uri, revision, len(dbSecretValues))
	}
	return nil, &coresecrets.ValueRef{
		BackendID:  dbSecretValueRefs[0].BackendUUID,
		RevisionID: dbSecretValueRefs[0].RevisionID,
	}, nil
}

// checkExistsIfLocal returns true of the secret is local to this model.
// It returns an error satisfying [secreterrors.SecretNotFound] if the
// specified secret URI is from this model and the secret it refers to
// does not exist in the model.
func (st State) checkExistsIfLocal(ctx context.Context, tx *sqlair.TX, uri *coresecrets.URI) (bool, error) {
	query := `
WITH local AS (
    SELECT 'local' AS is_local FROM secret_metadata sm
    WHERE  sm.secret_id = $secretRef.secret_id
    AND    ($secretRef.source_uuid = '' OR $secretRef.source_uuid = (SELECT uuid FROM model))
),
remote AS (
    SELECT 'remote' AS is_local FROM model
    WHERE $secretRef.source_uuid <> '' AND uuid <> $secretRef.source_uuid
)
SELECT is_local as &M.is_local
FROM (SELECT * FROM local UNION SELECT * FROM remote)`

	ref := secretRef{ID: uri.ID, SourceUUID: uri.SourceUUID}
	queryStmt, err := st.Prepare(query, ref, sqlair.M{})
	if err != nil {
		return false, errors.Trace(err)
	}
	result := sqlair.M{}
	err = tx.Query(ctx, queryStmt, ref).Get(&result)
	if err == nil {
		isLocal := result["is_local"]
		return isLocal == "local", nil
	}
	if errors.Is(err, sqlair.ErrNoRows) {
		return false, secreterrors.SecretNotFound
	}
	return false, errors.Annotatef(err, "looking up secret URI %q", uri)
}

// GetSecretConsumer returns the secret consumer info for the specified unit
// and secret, along withthe latest revision for the secret.
// If the unit does not exist, an error satisfying [uniterrors.NotFound] is
// returned.If the secret does not exist, an error satisfying
// [secreterrors.SecretNotFound] is returned.
// If there's not currently a consumer record for the secret, the latest
// revision is still returned,along with an error satisfying
// [secreterrors.SecretConsumerNotFound].
func (st State) GetSecretConsumer(
	ctx context.Context, uri *coresecrets.URI, unitName string,
) (*coresecrets.SecretConsumerMetadata, int, error) {
	db, err := st.DB()
	if err != nil {
		return nil, 0, errors.Trace(err)
	}

	consumer := secretUnitConsumer{
		SecretID: uri.ID,
	}

	query := `
SELECT suc.label AS &secretUnitConsumer.label,
       suc.current_revision AS &secretUnitConsumer.current_revision
FROM   secret_unit_consumer suc
WHERE  suc.secret_id = $secretUnitConsumer.secret_id
AND    suc.unit_uuid = $secretUnitConsumer.unit_uuid`

	queryStmt, err := st.Prepare(query, secretUnitConsumer{})
	if err != nil {
		return nil, 0, errors.Trace(err)
	}

	selectUnitUUID := `SELECT &unit.uuid FROM unit WHERE unit_id=$unit.unit_id`
	selectUnitUUIDStmt, err := st.Prepare(selectUnitUUID, unit{})
	if err != nil {
		return nil, 0, errors.Trace(err)
	}

	selectLatestLocalRevision := `
SELECT MAX(revision) AS &secretRef.revision
FROM   secret_revision rev
WHERE  rev.secret_id = $secretRef.secret_id`
	selectLatestLocalRevisionStmt, err := st.Prepare(selectLatestLocalRevision, secretRef{})
	if err != nil {
		return nil, 0, errors.Trace(err)
	}

	selectLatestRemoteRevision := `
SELECT latest_revision AS &secretRef.revision
FROM   secret_reference ref
WHERE  ref.secret_id = $secretRef.secret_id`
	selectLatestRemoteRevisionStmt, err := st.Prepare(selectLatestRemoteRevision, secretRef{})
	if err != nil {
		return nil, 0, errors.Trace(err)
	}

	var (
		dbSecretConsumers secretUnitConsumers
		latestRevision    int
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		isLocal, err := st.checkExistsIfLocal(ctx, tx, uri)
		if err != nil {
			return errors.Trace(err)
		}

		result := unit{}
		err = tx.Query(ctx, selectUnitUUIDStmt, unit{UnitName: unitName}).Get(&result)
		if err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return fmt.Errorf("unit %q not found%w", unitName, errors.Hide(uniterrors.NotFound))
			} else {
				return errors.Annotatef(err, "looking up unit UUID for %q", unitName)
			}
		}
		consumer.UnitUUID = result.UUID
		err = tx.Query(ctx, queryStmt, consumer).GetAll(&dbSecretConsumers)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Annotate(err, "querying secret consumers")
		}

		latest := secretRef{}
		latestRevisionStmt := selectLatestLocalRevisionStmt
		if !isLocal {
			latestRevisionStmt = selectLatestRemoteRevisionStmt
		}
		err = tx.Query(ctx, latestRevisionStmt, secretRef{ID: uri.ID}).Get(&latest)
		if err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				// Only return secret not found for local secrets.
				// For remote secrets we may not yet know the latest revision.
				if isLocal {
					return secreterrors.SecretNotFound
				}
			} else {
				return errors.Annotatef(err, "looking up latest revision for %q", uri.ID)
			}
		}
		latestRevision = latest.Revision

		return nil
	})
	if err != nil {
		return nil, 0, errors.Trace(domain.CoerceError(err))
	}
	if len(dbSecretConsumers) == 0 {
		return nil, latestRevision, fmt.Errorf("secret consumer for %q and unit %q%w", uri.ID, unitName, secreterrors.SecretConsumerNotFound)
	}
	consumers := dbSecretConsumers.toSecretConsumers()
	return consumers[0], latestRevision, nil
}

// SaveSecretConsumer saves the consumer metadata for the given secret and unit.
// If the unit does not exist, an error satisfying [uniterrors.NotFound] is returned.
// If the secret does not exist, an error satisfying [secreterrors.SecretNotFound] is returned.
func (st State) SaveSecretConsumer(
	ctx context.Context, uri *coresecrets.URI, unitName string, md *coresecrets.SecretConsumerMetadata,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	insertQuery := `
INSERT INTO secret_unit_consumer (*)
VALUES ($secretUnitConsumer.*)
ON CONFLICT(secret_id, unit_uuid) DO UPDATE SET
    label=excluded.label,
    current_revision=excluded.current_revision`

	insertStmt, err := st.Prepare(insertQuery, secretUnitConsumer{})
	if err != nil {
		return errors.Trace(err)
	}

	selectUnitUUID := `select &M.uuid FROM unit WHERE unit_id=$M.unit_id`
	selectUnitUUIDStmt, err := st.Prepare(selectUnitUUID, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}

	// We might be saving a tracked revision for a remote secret
	// before we have been notified of a revision change.
	// So we might need to insert the parent secret URI.
	insertRemoteSecretQuery := `
INSERT INTO secret (id)
VALUES ($secretID.id)
ON CONFLICT DO NOTHING`

	insertRemoteSecretStmt, err := st.Prepare(insertRemoteSecretQuery, secretID{})
	if err != nil {
		return errors.Trace(err)
	}

	consumer := secretUnitConsumer{
		SecretID:        uri.ID,
		SourceModelUUID: uri.SourceUUID,
		Label:           md.Label,
		CurrentRevision: md.CurrentRevision,
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		isLocal, err := st.checkExistsIfLocal(ctx, tx, uri)
		if err != nil {
			return errors.Trace(err)
		}

		if !isLocal {
			// Ensure a remote secret parent URI is recorded. This will normally
			// be done by the watcher but it may not have fired yet.
			err = tx.Query(ctx, insertRemoteSecretStmt, secretID{ID: uri.ID}).Run()
			if err != nil {
				return errors.Annotatef(err, "inserting secret reference for %q", uri)
			}
		}

		result := sqlair.M{}
		err = tx.Query(ctx, selectUnitUUIDStmt, sqlair.M{"unit_id": unitName}).Get(&result)
		if err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return fmt.Errorf("unit %q not found%w", unitName, errors.Hide(uniterrors.NotFound))
			} else {
				return errors.Annotatef(err, "looking up unit UUID for %q", unitName)
			}
		}
		consumer.UnitUUID = result["uuid"].(string)
		if err := tx.Query(ctx, insertStmt, consumer).Run(); err != nil {
			return errors.Trace(err)
		}

		if err := st.markObsoleteRevisions(ctx, tx, uri); err != nil {
			return errors.Annotatef(err, "marking obsolete revisions for secret %q", uri)
		}

		return nil
	})
	return errors.Trace(domain.CoerceError(err))
}

// GetSecretRemoteConsumer returns the secret consumer info from a cross model consumer
// for the specified unit and secret.
// If the secret does not exist, an error satisfying [secreterrors.SecretNotFound] is returned.
// If there's not currently a consumer record for the secret, the latest revision is still returned,
// along with an error satisfying [secreterrors.SecretConsumerNotFound].
func (st State) GetSecretRemoteConsumer(
	ctx context.Context, uri *coresecrets.URI, unitName string,
) (*coresecrets.SecretConsumerMetadata, int, error) {
	db, err := st.DB()
	if err != nil {
		return nil, 0, errors.Trace(err)
	}

	consumer := secretRemoteUnitConsumer{
		SecretID: uri.ID,
		UnitID:   unitName,
	}

	query := `
SELECT suc.current_revision AS &secretRemoteUnitConsumer.current_revision
FROM   secret_remote_unit_consumer suc
WHERE  suc.secret_id = $secretRemoteUnitConsumer.secret_id
AND    suc.unit_id = $secretRemoteUnitConsumer.unit_id`

	queryStmt, err := st.Prepare(query, secretRemoteUnitConsumer{})
	if err != nil {
		return nil, 0, errors.Trace(err)
	}

	selectLatestRevision := `
SELECT MAX(revision) AS &secretInfo.latest_revision
FROM   secret_revision rev
WHERE  rev.secret_id = $secretInfo.secret_id`
	selectLatestRevisionStmt, err := st.Prepare(selectLatestRevision, secretInfo{})
	if err != nil {
		return nil, 0, errors.Trace(err)
	}

	var (
		dbSecretConsumers secretRemoteUnitConsumers
		latestRevision    int
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if isLocal, err := st.checkExistsIfLocal(ctx, tx, uri); err != nil {
			return errors.Trace(err)
		} else if !isLocal {
			// Should never happen.
			return secreterrors.SecretIsNotLocal
		}

		err = tx.Query(ctx, queryStmt, consumer).GetAll(&dbSecretConsumers)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Annotatef(err, "querying secret consumer info for secret %q and unit %q", uri, unitName)
		}

		result := secretInfo{ID: uri.ID}
		err = tx.Query(ctx, selectLatestRevisionStmt, result).Get(&result)
		if err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return secreterrors.SecretNotFound
			} else {
				return errors.Annotatef(err, "looking up latest revision for %q", uri.ID)
			}
		}
		latestRevision = result.LatestRevision

		return nil
	})
	if err != nil {
		return nil, 0, errors.Trace(domain.CoerceError(err))
	}
	if len(dbSecretConsumers) == 0 {
		return nil, latestRevision, fmt.Errorf(
			"secret consumer for %q and unit %q%w", uri.ID, unitName, secreterrors.SecretConsumerNotFound)
	}
	consumers := dbSecretConsumers.toSecretConsumers()
	return consumers[0], latestRevision, nil
}

// SaveSecretRemoteConsumer saves the consumer metadata for the given secret and unit.
// If the secret does not exist, an error satisfying [secreterrors.SecretNotFound] is returned.
func (st State) SaveSecretRemoteConsumer(
	ctx context.Context, uri *coresecrets.URI, unitName string, md *coresecrets.SecretConsumerMetadata,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	insertQuery := `
INSERT INTO secret_remote_unit_consumer (*)
VALUES ($secretRemoteUnitConsumer.*)
ON CONFLICT(secret_id, unit_id) DO UPDATE SET
    current_revision=excluded.current_revision`

	insertStmt, err := st.Prepare(insertQuery, secretRemoteUnitConsumer{})
	if err != nil {
		return errors.Trace(err)
	}

	consumer := secretRemoteUnitConsumer{
		SecretID:        uri.ID,
		UnitID:          unitName,
		CurrentRevision: md.CurrentRevision,
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if isLocal, err := st.checkExistsIfLocal(ctx, tx, uri); err != nil {
			return errors.Trace(err)
		} else if !isLocal {
			// Should never happen.
			return secreterrors.SecretIsNotLocal
		}
		if err := tx.Query(ctx, insertStmt, consumer).Run(); err != nil {
			return errors.Trace(err)
		}

		if err := st.markObsoleteRevisions(ctx, tx, uri); err != nil {
			return errors.Annotatef(err, "marking obsolete revisions for secret %q", uri)
		}

		return nil
	})
	return errors.Trace(domain.CoerceError(err))
}

// UpdateRemoteSecretRevision records the latest revision
// of the specified cross model secret.
func (st State) UpdateRemoteSecretRevision(ctx context.Context, uri *coresecrets.URI, latestRevision int) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	insertQuery := `
INSERT INTO secret (id)
VALUES ($secretID.id)
ON CONFLICT(id) DO NOTHING`

	insertStmt, err := st.Prepare(insertQuery, secretID{})
	if err != nil {
		return errors.Trace(err)
	}

	insertLatestQuery := `
INSERT INTO secret_reference (*)
VALUES ($remoteSecret.*)
ON CONFLICT(secret_id) DO UPDATE SET
    latest_revision=excluded.latest_revision`

	insertLatestStmt, err := st.Prepare(insertLatestQuery, remoteSecret{})
	if err != nil {
		return errors.Trace(err)
	}

	secret := remoteSecret{
		SecretID:       uri.ID,
		LatestRevision: latestRevision,
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, insertStmt, secretID{ID: uri.ID}).Run()
		if err != nil {
			return errors.Annotatef(err, "inserting URI record for cross model secret %q", uri)
		}
		if err := tx.Query(ctx, insertLatestStmt, secret).Run(); err != nil {
			return errors.Annotatef(err, "updating latest revision %d for cross model secret %q", latestRevision, uri)
		}
		if err := st.markObsoleteRevisions(ctx, tx, uri); err != nil {
			return errors.Annotatef(err, "marking obsolete revisions for secret %q", uri)
		}
		return nil
	})
	return errors.Trace(domain.CoerceError(err))
}

// GrantAccess grants access to the secret for the specified subject with the specified scope.
// It returns an error satisfying [secreterrors.SecretNotFound] if the secret is not found.
// If an attempt is made to change an existing permission's scope or subject type, an error
// satisfying [secreterrors.InvalidSecretPermissionChange] is returned.
func (st State) GrantAccess(ctx context.Context, uri *coresecrets.URI, params domainsecret.GrantParams) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	checkInvariantQuery := `
SELECT sp.secret_id AS &secretID.id
FROM   secret_permission sp
WHERE  sp.secret_id = $secretPermission.secret_id
AND    sp.subject_uuid = $secretPermission.subject_uuid
AND    (sp.subject_type_id <> $secretPermission.subject_type_id
        OR sp.scope_uuid <> $secretPermission.scope_uuid
        OR sp.scope_type_id <> $secretPermission.scope_type_id)`

	checkInvariantStmt, err := st.Prepare(checkInvariantQuery, secretPermission{}, secretID{})
	if err != nil {
		return errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		perm := secretPermission{
			SecretID: uri.ID,
			RoleID:   params.RoleID,
		}
		if isLocal, err := st.checkExistsIfLocal(ctx, tx, uri); err != nil {
			return errors.Trace(err)
		} else if !isLocal {
			// Should never happen.
			return secreterrors.SecretIsNotLocal
		}

		// Look up the UUID of the subject.
		perm.SubjectTypeID = params.SubjectTypeID
		perm.SubjectUUID, err = st.lookupSubjectUUID(ctx, tx, params.SubjectID, params.SubjectTypeID)
		if err != nil {
			return errors.Trace(err)
		}

		// Look up the UUID of the access scope entity.
		perm.ScopeTypeID = params.ScopeTypeID
		perm.ScopeUUID, err = st.lookupScopeUUID(ctx, tx, params.ScopeID, params.ScopeTypeID)
		if err != nil {
			return errors.Trace(err)
		}

		// Check that the access scope or subject type is not changing.
		id := secretID{}
		err = tx.Query(ctx, checkInvariantStmt, perm).Get(&id)
		if err == nil {
			// Should never happen.
			return secreterrors.InvalidSecretPermissionChange
		} else if !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Annotatef(err, "checking duplicate permission record for secret %q", uri)
		}

		return st.grantAccess(ctx, tx, perm)
	})
	return errors.Trace(domain.CoerceError(err))
}

const (
	selectUnitUUID        = `SELECT uuid AS &entityRef.uuid FROM unit WHERE unit_id=$entityRef.id`
	selectApplicationUUID = `SELECT uuid AS &entityRef.uuid FROM application WHERE name=$entityRef.id`
	selectModelUUID       = `SELECT uuid AS &entityRef.uuid FROM model WHERE uuid=$entityRef.id`
)

func (st State) lookupSubjectUUID(
	ctx context.Context, tx *sqlair.TX, subjectID string, subjectTypeID domainsecret.GrantSubjectType,
) (string, error) {
	var (
		selectSubjectUUID        string
		selectSubjectQueryParams = []any{entityRef{ID: subjectID}}
		subjectNotFoundError     error
	)
	switch subjectTypeID {
	case domainsecret.SubjectUnit:
		selectSubjectUUID = selectUnitUUID
		subjectNotFoundError = uniterrors.NotFound
	case domainsecret.SubjectApplication:
		selectSubjectUUID = selectApplicationUUID
		subjectNotFoundError = applicationerrors.ApplicationNotFound
	case domainsecret.SubjectRemoteApplication:
		// TODO(secrets) - we don't have remote applications in dqlite yet
		// Just use a temporary query that returns the id as uuid.
		selectSubjectUUID = "SELECT uuid AS &entityRef.uuid FROM (SELECT $M.subject_id AS uuid FROM model) WHERE $entityRef.id <> ''"
		selectSubjectQueryParams = append(selectSubjectQueryParams, sqlair.M{"subject_id": subjectID})
		subjectNotFoundError = applicationerrors.ApplicationNotFound
	case domainsecret.SubjectModel:
		selectSubjectUUID = selectModelUUID
		subjectNotFoundError = modelerrors.NotFound
	}
	selectSubjectUUIDStmt, err := st.Prepare(selectSubjectUUID, selectSubjectQueryParams...)
	if err != nil {
		return "", errors.Trace(err)
	}
	result := entityRef{}
	err = tx.Query(ctx, selectSubjectUUIDStmt, selectSubjectQueryParams...).Get(&result)
	if err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return "", fmt.Errorf("%s %q not found%w", subjectTypeID, subjectID, errors.Hide(subjectNotFoundError))
		} else {
			subject := subjectID
			if subjectTypeID == domainsecret.SubjectModel {
				subject = "model"
			}
			return "", errors.Annotatef(err, "looking up secret grant subject UUID for %q", subject)
		}
	}
	return result.UUID, nil
}

func (st State) lookupScopeUUID(
	ctx context.Context, tx *sqlair.TX, scopeID string, scopeTypeID domainsecret.GrantScopeType,
) (string, error) {
	var (
		selectScopeUUID        string
		selectScopeQueryParams = []any{entityRef{ID: scopeID}}
		scopeNotFoundError     error
	)
	switch scopeTypeID {
	case domainsecret.ScopeUnit:
		selectScopeUUID = selectUnitUUID
		scopeNotFoundError = uniterrors.NotFound
	case domainsecret.ScopeApplication:
		selectScopeUUID = selectApplicationUUID
		scopeNotFoundError = applicationerrors.ApplicationNotFound
	case domainsecret.ScopeModel:
		selectScopeUUID = selectModelUUID
		scopeNotFoundError = modelerrors.NotFound
	case domainsecret.ScopeRelation:
		// TODO(secrets) - we don't have relations in dqlite yet
		// Just use a temporary query that returns the id as uuid.
		selectScopeUUID = "SELECT uuid AS &entityRef.uuid FROM (SELECT $M.scope_id AS uuid FROM model) WHERE $entityRef.id <> ''"
		selectScopeQueryParams = append(selectScopeQueryParams, sqlair.M{"scope_id": scopeID})
	}
	selectScopeUUIDStmt, err := st.Prepare(selectScopeUUID, selectScopeQueryParams...)
	if err != nil {
		return "", errors.Trace(err)
	}

	result := entityRef{}
	err = tx.Query(ctx, selectScopeUUIDStmt, selectScopeQueryParams...).Get(&result)
	if err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return "", fmt.Errorf("%s %q not found%w", scopeTypeID, scopeID, errors.Hide(scopeNotFoundError))
		} else {
			scope := scopeID
			if scopeTypeID == domainsecret.ScopeModel {
				scope = "model"
			}
			return "", errors.Annotatef(err, "looking up secret grant scope UUID for %q", scope)
		}
	}
	return result.UUID, nil
}

func (st State) grantAccess(ctx context.Context, tx *sqlair.TX, perm secretPermission) error {
	insertQuery := `
INSERT INTO secret_permission (*)
VALUES ($secretPermission.*)
ON CONFLICT(secret_id, subject_uuid) DO UPDATE SET
    role_id=excluded.role_id,
    -- These are needed to fire the immutable trigger.
    subject_type_id=excluded.subject_type_id,
    scope_type_id=excluded.scope_type_id,
    scope_uuid=excluded.scope_uuid`

	insertStmt, err := st.Prepare(insertQuery, secretPermission{})
	if err != nil {
		return errors.Trace(err)
	}

	if err := tx.Query(ctx, insertStmt, perm).Run(); err != nil {
		return errors.Trace(err)
	}
	return nil

}

// RevokeAccess revokes access to the secret for the specified subject.
// It returns an error satisfying [secreterrors.SecretNotFound] if the
// secret is not found.
func (st State) RevokeAccess(ctx context.Context, uri *coresecrets.URI, params domainsecret.AccessParams) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	deleteQuery := `
DELETE FROM secret_permission
WHERE  secret_id = $secretPermission.secret_id
AND    subject_type_id = $secretPermission.subject_type_id
AND    subject_uuid = $secretPermission.subject_uuid`

	perm := secretPermission{
		SecretID:      uri.ID,
		SubjectTypeID: params.SubjectTypeID,
	}
	deleteStmt, err := st.Prepare(deleteQuery, perm)
	if err != nil {
		return errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if isLocal, err := st.checkExistsIfLocal(ctx, tx, uri); err != nil {
			return errors.Trace(err)
		} else if !isLocal {
			// Should never happen.
			return secreterrors.SecretIsNotLocal
		}

		// Look up the UUID of the subject.
		perm.SubjectUUID, err = st.lookupSubjectUUID(ctx, tx, params.SubjectID, params.SubjectTypeID)
		if err != nil {
			return errors.Trace(err)
		}
		err = tx.Query(ctx, deleteStmt, perm).Run()
		return errors.Annotatef(err, "deleting secret grant for %q on %q", params.SubjectID, uri)
	})
	return errors.Trace(domain.CoerceError(err))
}

// GetSecretAccess returns the access to the secret for the specified accessor.
// It returns an error satisfying [secreterrors.SecretNotFound]
// if the secret is not found.
func (st State) GetSecretAccess(
	ctx context.Context, uri *coresecrets.URI, params domainsecret.AccessParams,
) (string, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Trace(err)
	}

	query := `
SELECT sr.role AS &M.role
FROM   v_secret_permission sp
       JOIN secret_role sr ON sr.id = sp.role_id
WHERE  secret_id = $secretAccessor.secret_id
AND    subject_type_id = $secretAccessor.subject_type_id
AND    subject_id = $secretAccessor.subject_id`

	access := secretAccessor{
		SecretID:      uri.ID,
		SubjectTypeID: params.SubjectTypeID,
		SubjectID:     params.SubjectID,
	}
	selectRoleStmt, err := st.Prepare(query, access, sqlair.M{})
	if err != nil {
		return "", errors.Trace(err)
	}

	var role string
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if isLocal, err := st.checkExistsIfLocal(ctx, tx, uri); err != nil {
			return errors.Trace(err)
		} else if !isLocal {
			// Should never happen.
			return secreterrors.SecretIsNotLocal
		}
		result := sqlair.M{}
		err = tx.Query(ctx, selectRoleStmt, access).Get(&result)
		if err == nil || errors.Is(err, sqlair.ErrNoRows) {
			role, _ = result["role"].(string)
			return nil
		}
		return errors.Annotatef(err, "looking up secret grant for %q on %q", params.SubjectID, uri)
	})
	return role, errors.Trace(domain.CoerceError(err))
}

// GetSecretAccessScope returns the access scope for the specified accessor's
// permission on the secret.It returns an error satisfying
// [secreterrors.SecretNotFound] if the secret is not found.
func (st State) GetSecretAccessScope(
	ctx context.Context, uri *coresecrets.URI, params domainsecret.AccessParams,
) (*domainsecret.AccessScope, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	query := `
SELECT (sp.scope_id, sp.scope_type_id) AS (&secretAccessScope.*)
FROM   v_secret_permission sp
WHERE  secret_id = $secretAccessor.secret_id
AND    subject_type_id = $secretAccessor.subject_type_id
AND    subject_id = $secretAccessor.subject_id`

	access := secretAccessor{
		SecretID:      uri.ID,
		SubjectTypeID: params.SubjectTypeID,
		SubjectID:     params.SubjectID,
	}
	selectScopeStmt, err := st.Prepare(query, access, secretAccessScope{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	result := secretAccessScope{}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if isLocal, err := st.checkExistsIfLocal(ctx, tx, uri); err != nil {
			return errors.Trace(err)
		} else if !isLocal {
			// Should never happen.
			return secreterrors.SecretIsNotLocal
		}
		err = tx.Query(ctx, selectScopeStmt, access).Get(&result)
		if errors.Is(err, sqlair.ErrNoRows) {
			return fmt.Errorf(
				"access scope for %q on secret %q not found%w",
				params.SubjectID, uri, errors.Hide(secreterrors.SecretAccessScopeNotFound))
		}
		return errors.Annotatef(err, "looking up secret access scope for %q on %q", params.SubjectID, uri)
	})
	if err != nil {
		return nil, errors.Trace(domain.CoerceError(err))
	}
	return &domainsecret.AccessScope{
		ScopeTypeID: result.ScopeTypeID,
		ScopeID:     result.ScopeID,
	}, nil
}

// GetSecretGrants returns the subjects which have the specified access to the secret.
// It returns an error satisfying [secreterrors.SecretNotFound] if the secret is not found.
func (st State) GetSecretGrants(
	ctx context.Context, uri *coresecrets.URI, role coresecrets.SecretRole,
) ([]domainsecret.GrantParams, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	query := `
SELECT (sp.*) AS (&secretAccessor.*),
       (sp.*) AS (&secretAccessScope.*)
FROM   v_secret_permission sp
WHERE  secret_id = $secretID.id
AND    role_id = $secretAccessor.role_id
-- exclude remote applications
AND    subject_type_id != $M.remote_application_type`

	selectStmt, err := st.Prepare(query, secretID{}, secretAccessor{}, secretAccessScope{}, sqlair.M{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	secretIDParam := secretID{
		ID: uri.ID,
	}
	secretRole := secretAccessor{
		RoleID: domainsecret.MarshallRole(role),
	}

	var (
		accessors    secretAccessors
		accessScopes secretAccessScopes
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if isLocal, err := st.checkExistsIfLocal(ctx, tx, uri); err != nil {
			return errors.Trace(err)
		} else if !isLocal {
			// Should never happen.
			return secreterrors.SecretIsNotLocal
		}
		err = tx.Query(ctx, selectStmt, secretIDParam, secretRole, sqlair.M{
			"remote_application_type": domainsecret.SubjectRemoteApplication,
		}).GetAll(&accessors, &accessScopes)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return errors.Annotatef(err, "looking up secret grants for %q", uri)
	})
	if err != nil {
		return nil, errors.Trace(domain.CoerceError(err))
	}
	return accessors.toSecretGrants(accessScopes)
}

type (
	units        []string
	applications []string
	models       []string
)

// ListGrantedSecretsForBackend returns the secret revision info for any
// secrets from the specified backend for which the specified consumers
// have been granted the specified access.
func (st State) ListGrantedSecretsForBackend(
	ctx context.Context, backendID string, accessors []domainsecret.AccessParams, role coresecrets.SecretRole,
) ([]*coresecrets.SecretRevisionRef, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	query := `
SELECT (sm.secret_id) AS (&secretInfo.*),
       (svr.*) AS (&secretValueRef.*)
FROM   secret_metadata sm
       JOIN secret_revision rev ON rev.secret_id = sm.secret_id
       JOIN secret_value_ref svr ON svr.revision_uuid = rev.uuid
       JOIN v_secret_permission sp ON sp.secret_id = sm.secret_id
WHERE  sp.role_id = $secretAccessor.role_id
AND    svr.backend_uuid = $secretBackendID.id
AND    (subject_type_id = $secretAccessorType.unit_type_id AND subject_id IN ($units[:])
        OR subject_type_id = $secretAccessorType.app_type_id AND subject_id IN ($applications[:])
        OR subject_type_id = $secretAccessorType.model_type_id AND subject_id IN ($models[:])
       )`
	secretBackendID := secretBackendID{
		ID: backendID,
	}

	secretRole := secretAccessor{
		RoleID: domainsecret.MarshallRole(role),
	}

	// Ideally we'd use IN tuple but sqlair doesn't support that.
	var (
		modelAccessors models
		appAccessors   applications
		unitAccessors  units
	)
	for _, a := range accessors {
		switch a.SubjectTypeID {
		case domainsecret.SubjectUnit:
			unitAccessors = append(unitAccessors, a.SubjectID)
		case domainsecret.SubjectApplication:
			appAccessors = append(appAccessors, a.SubjectID)
		case domainsecret.SubjectModel:
			modelAccessors = append(modelAccessors, a.SubjectID)
		default:
			continue
		}
	}

	queryParams := []any{
		appAccessors,
		unitAccessors,
		modelAccessors,
		secretAccessorTypeParam,
		secretRole,
		secretBackendID,
	}

	queryStmt, err := st.Prepare(query, append([]any{secretInfo{}, secretValueRef{}}, queryParams...)...)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var revisionResult []*coresecrets.SecretRevisionRef
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		var (
			dbSecrets   secrets
			dbValueRefs secretValueRefs
		)
		err = tx.Query(ctx, queryStmt, queryParams...).GetAll(&dbSecrets, &dbValueRefs)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Annotate(err, "querying accessible secrets")
		}
		revisionResult, err = dbSecrets.toSecretRevisionRef(dbValueRefs)
		return errors.Trace(err)
	}); err != nil {
		return nil, errors.Trace(domain.CoerceError(err))
	}
	return revisionResult, nil
}

type dbrevisionUUIDs []revisionUUID

// InitialWatchStatementForConsumedSecretsChange returns the initial watch
// statement and the table name for watching consumed secrets.
func (st State) InitialWatchStatementForConsumedSecretsChange(unitName string) (string, eventsource.NamespaceQuery) {
	queryFunc := func(ctx context.Context, runner coredatabase.TxnRunner) ([]string, error) {
		q := `
SELECT   DISTINCT sr.uuid AS &revisionUUID.uuid
FROM     secret_unit_consumer suc
         JOIN unit u ON u.uuid = suc.unit_uuid
         JOIN secret_revision sr ON sr.secret_id = suc.secret_id
WHERE    u.unit_id = $unit.unit_id
GROUP BY sr.secret_id
HAVING   suc.current_revision < MAX(sr.revision)`

		queryParams := []any{
			unit{UnitName: unitName},
		}

		stmt, err := st.Prepare(q, append([]any{revisionUUID{}}, queryParams...)...)
		if err != nil {
			return nil, errors.Trace(err)
		}

		var revUUIDs dbrevisionUUIDs
		err = runner.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			err := tx.Query(ctx, stmt, queryParams...).GetAll(&revUUIDs)
			if errors.Is(err, sqlair.ErrNoRows) {
				// No consumed secrets found.
				return nil
			}
			return errors.Trace(err)
		})
		if err != nil {
			return nil, errors.Trace(domain.CoerceError(err))
		}

		result := make([]string, len(revUUIDs))
		for i, rev := range revUUIDs {
			result[i] = rev.UUID
		}
		return result, nil
	}
	return "secret_revision", queryFunc
}

// GetConsumedSecretURIsWithChanges returns the URIs of the secrets
// consumed by the specified unit that has new revisions.
func (st State) GetConsumedSecretURIsWithChanges(
	ctx context.Context, unitName string, revisionIDs ...string,
) ([]string, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	q := `
SELECT DISTINCT suc.secret_id AS &secretUnitConsumer.secret_id
FROM   secret_unit_consumer suc
       JOIN unit u ON u.uuid = suc.unit_uuid
       JOIN secret_revision sr ON sr.secret_id = suc.secret_id
WHERE  u.unit_id = $unit.unit_id`

	queryParams := []any{
		unit{UnitName: unitName},
	}

	if len(revisionIDs) > 0 {
		queryParams = append(queryParams, revisionUUIDs(revisionIDs))
		q += " AND sr.uuid IN ($revisionUUIDs[:])"
	}
	q += `
GROUP BY sr.secret_id
HAVING suc.current_revision < MAX(sr.revision)`

	stmt, err := st.Prepare(q, append([]any{secretUnitConsumer{}}, queryParams...)...)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var dbConsumers secretUnitConsumers
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, queryParams...).GetAll(&dbConsumers)
		if errors.Is(err, sqlair.ErrNoRows) {
			// No consumed secrets found.
			return nil
		}
		return errors.Trace(err)
	})
	if err != nil {
		return nil, errors.Trace(domain.CoerceError(err))
	}

	secretURIs := make([]string, len(dbConsumers))
	for i, consumer := range dbConsumers {
		uri, err := coresecrets.ParseURI(consumer.SecretID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		secretURIs[i] = uri.String()
	}
	return secretURIs, nil
}

type remoteSecrets []remoteSecret

// InitialWatchStatementForConsumedRemoteSecretsChange returns the initial
// watch statement and the table name for watching consumed secrets hosted
// in a different model.
func (st State) InitialWatchStatementForConsumedRemoteSecretsChange(unitName string) (string, eventsource.NamespaceQuery) {
	queryFunc := func(ctx context.Context, runner coredatabase.TxnRunner) ([]string, error) {
		q := `
SELECT   DISTINCT sr.secret_id AS &remoteSecret.secret_id
FROM     secret_unit_consumer suc
         JOIN unit u ON u.uuid = suc.unit_uuid
         JOIN secret_reference sr ON sr.secret_id = suc.secret_id
WHERE    u.unit_id = $unit.unit_id
GROUP BY sr.secret_id
HAVING   suc.current_revision < sr.latest_revision`

		queryParams := []any{
			unit{UnitName: unitName},
		}

		stmt, err := st.Prepare(q, append([]any{remoteSecret{}}, queryParams...)...)
		if err != nil {
			return nil, errors.Trace(err)
		}
		var referenceIDs remoteSecrets
		err = runner.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			err := tx.Query(ctx, stmt, queryParams...).GetAll(&referenceIDs)
			if errors.Is(err, sqlair.ErrNoRows) {
				// No consumed remote secrets found.
				return nil
			}
			return errors.Trace(err)
		})
		if err != nil {
			return nil, errors.Trace(domain.CoerceError(err))
		}

		result := make([]string, len(referenceIDs))
		for i, rev := range referenceIDs {
			result[i] = rev.SecretID
		}
		return result, nil
	}
	return "secret_reference", queryFunc
}

// GetConsumedRemoteSecretURIsWithChanges returns the URIs of the secrets
// consumed by the specified unit that have new revisions and are hosted
// on a different model.
func (st State) GetConsumedRemoteSecretURIsWithChanges(
	ctx context.Context, unitName string, secretIDs ...string,
) ([]string, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	q := `
SELECT suc.secret_id AS &secretUnitConsumer.secret_id,
       suc.source_model_uuid AS &secretUnitConsumer.source_model_uuid
FROM   secret_unit_consumer suc
       JOIN unit u ON u.uuid = suc.unit_uuid
       JOIN secret_reference sr ON sr.secret_id = suc.secret_id
WHERE  u.unit_id = $unit.unit_id`

	queryParams := []any{
		unit{UnitName: unitName},
	}

	if len(secretIDs) > 0 {
		queryParams = append(queryParams, dbSecretIDs(secretIDs))
		q += " AND sr.secret_id IN ($dbSecretIDs[:])"
	}
	q += `
GROUP BY sr.secret_id
HAVING suc.current_revision < sr.latest_revision`

	stmt, err := st.Prepare(q, append([]any{secretUnitConsumer{}}, queryParams...)...)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var consumers secretUnitConsumers
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, queryParams...).GetAll(&consumers)
		if errors.Is(err, sqlair.ErrNoRows) {
			// No consumed secrets found.
			return nil
		}
		return errors.Trace(err)
	})
	if err != nil {
		return nil, errors.Trace(domain.CoerceError(err))
	}

	secretURIs := make([]string, len(consumers))
	for i, consumer := range consumers {
		uri, err := coresecrets.ParseURI(consumer.SecretID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		uri.SourceUUID = consumer.SourceModelUUID
		secretURIs[i] = uri.String()
	}
	return secretURIs, nil
}

// InitialWatchStatementForRemoteConsumedSecretsChangesFromOfferingSide
// returns the initial watch statement and the table name for watching
// remote consumed secrets.
func (st State) InitialWatchStatementForRemoteConsumedSecretsChangesFromOfferingSide(
	appName string,
) (string, eventsource.NamespaceQuery) {
	queryFunc := func(ctx context.Context, runner coredatabase.TxnRunner) ([]string, error) {

		// TODO: sqlair does not support inject parameters into values in quotation marks.
		// Use sqlair to generate the query once https://github.com/canonical/sqlair/issues/148 is fixed.
		// q := `
		// SELECT DISTINCT sr.uuid AS &revisionUUID.uuid
		// FROM secret_remote_unit_consumer sruc
		// LEFT JOIN secret_revision sr ON sr.secret_id = sruc.secret_id
		// WHERE sruc.unit_id LIKE '$M.app_name/%'`

		q := fmt.Sprintf(`
SELECT DISTINCT sr.uuid AS &revisionUUID.uuid
FROM   secret_remote_unit_consumer sruc
       LEFT JOIN secret_revision sr ON sr.secret_id = sruc.secret_id
WHERE  sruc.unit_id LIKE '%s/%%'`, appName)

		queryParams := []any{
			// TODO: enable this once https://github.com/canonical/sqlair/issues/148 is fixed.
			// sqlair.M{"app_name": appName},
		}
		q += `
GROUP BY sruc.secret_id
HAVING sruc.current_revision < MAX(sr.revision)`
		stmt, err := st.Prepare(q, append([]any{revisionUUID{}}, queryParams...)...)
		if err != nil {
			return nil, errors.Trace(err)
		}
		var revisionUUIDs dbrevisionUUIDs
		err = runner.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			err := tx.Query(ctx, stmt, queryParams...).GetAll(&revisionUUIDs)
			if errors.Is(err, sqlair.ErrNoRows) {
				// No consumed secrets found.
				return nil
			}
			return errors.Trace(err)
		})
		if err != nil {
			return nil, errors.Trace(domain.CoerceError(err))
		}
		revUUIDs := make([]string, len(revisionUUIDs))
		for i, rev := range revisionUUIDs {
			revUUIDs[i] = rev.UUID
		}
		return revUUIDs, nil
	}
	return "secret_revision", queryFunc
}

// GetRemoteConsumedSecretURIsWithChangesFromOfferingSide returns the URIs
// of the secrets consumed by the specified remote application that has new
// revisions.
func (st State) GetRemoteConsumedSecretURIsWithChangesFromOfferingSide(
	ctx context.Context, appName string, revUUIDs ...string,
) ([]string, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}
	// TODO: sqlair does not support inject parameters into values in quotation marks.
	// Use sqlair to generate the query once https://github.com/canonical/sqlair/issues/148 is fixed.
	// q := `
	// SELECT DISTINCT sruc.secret_id AS &secretRemoteUnitConsumer.secret_id
	// FROM secret_remote_unit_consumer sruc
	// LEFT JOIN secret_revision sr ON sr.secret_id = sruc.secret_id
	// WHERE sruc.unit_id LIKE '$M.app_name/%'`

	q := fmt.Sprintf(`
SELECT DISTINCT sruc.secret_id AS &secretRemoteUnitConsumer.secret_id
FROM   secret_remote_unit_consumer sruc
       LEFT JOIN secret_revision sr ON sr.secret_id = sruc.secret_id
WHERE  sruc.unit_id LIKE '%s/%%'`, appName)

	queryParams := []any{
		// TODO: enable this once https://github.com/canonical/sqlair/issues/148 is fixed.
		// sqlair.M{"app_name": appName},
	}
	if len(revUUIDs) > 0 {
		queryParams = append(queryParams, revisionUUIDs(revUUIDs))
		q += " AND sr.uuid IN ($revisionUUIDs[:])"
	}
	q += `
GROUP BY sruc.secret_id
HAVING sruc.current_revision < MAX(sr.revision)`
	stmt, err := st.Prepare(q, append([]any{secretRemoteUnitConsumer{}}, queryParams...)...)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var remoteConsumers secretRemoteUnitConsumers
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, queryParams...).GetAll(&remoteConsumers)
		if errors.Is(err, sqlair.ErrNoRows) {
			// No consumed secrets found.
			return nil
		}
		return errors.Trace(err)
	})
	if err != nil {
		return nil, errors.Trace(domain.CoerceError(err))
	}
	modelUUID, err := st.GetModelUUID(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	secretURIs := make([]string, len(remoteConsumers))
	for i, consumer := range remoteConsumers {
		uri, err := coresecrets.ParseURI(consumer.SecretID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		// We need to set the source model UUID to mark it as a remote secret for comsumer side to use.
		uri.SourceUUID = modelUUID
		secretURIs[i] = uri.String()
	}
	return secretURIs, nil
}

type dbSecretIDs []string

// InitialWatchStatementForObsolete returns the initial watch statement
// and the table name for watching obsolete revisions.
func (st State) InitialWatchStatementForObsoleteRevision(
	appOwners domainsecret.ApplicationOwners, unitOwners domainsecret.UnitOwners,
) (string, eventsource.NamespaceQuery) {
	queryFunc := func(ctx context.Context, runner coredatabase.TxnRunner) ([]string, error) {
		var revisions []secretRevision
		if err := st.getRevisionForObsolete(
			ctx, runner, "sro.revision_uuid AS &secretRevision.uuid", secretRevision{}, &revisions,
			appOwners, unitOwners,
		); err != nil {
			return nil, errors.Trace(err)
		}
		revUUIDs := make([]string, len(revisions))
		for i, rev := range revisions {
			revUUIDs[i] = rev.ID
		}
		return revUUIDs, nil
	}
	return "secret_revision_obsolete", queryFunc
}

// GetRevisionIDsForObsolete filters the revision IDs that are obsolete and
// owned by the specified owners.Either revisionUUIDs, appOwners,
// or unitOwners must be specified.
func (st State) GetRevisionIDsForObsolete(
	ctx context.Context,
	appOwners domainsecret.ApplicationOwners,
	unitOwners domainsecret.UnitOwners,
	revisionUUIDs ...string,
) ([]string, error) {
	if len(revisionUUIDs) == 0 && len(appOwners) == 0 && len(unitOwners) == 0 {
		return nil, nil
	}
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var rows obsoleteRevisionRows
	if err := st.getRevisionForObsolete(
		ctx, db,
		"(sr.revision, sr.secret_id) AS (&obsoleteRevisionRow.*)", obsoleteRevisionRow{}, &rows,
		appOwners, unitOwners, revisionUUIDs...,
	); err != nil {
		return nil, errors.Trace(err)
	}
	return rows.toRevIDs(), nil
}

func (st State) getRevisionForObsolete(
	ctx context.Context, runner coredatabase.TxnRunner,
	selectStmt string,
	outputType, result any,
	appOwners domainsecret.ApplicationOwners,
	unitOwners domainsecret.UnitOwners,
	revUUIDs ...string,
) error {
	if len(revUUIDs) == 0 && len(appOwners) == 0 && len(unitOwners) == 0 {
		return nil
	}

	q := fmt.Sprintf(`
SELECT
    %s
FROM secret_revision_obsolete sro
     JOIN secret_revision sr ON sr.uuid = sro.revision_uuid`, selectStmt)

	var queryParams []any
	var joins []string
	conditions := []string{
		"sro.obsolete = true",
	}
	if len(revUUIDs) > 0 {
		queryParams = append(queryParams, revisionUUIDs(revUUIDs))
		conditions = append(conditions, "AND sr.uuid IN ($revisionUUIDs[:])")
	}
	if len(appOwners) > 0 && len(unitOwners) > 0 {
		queryParams = append(queryParams, appOwners, unitOwners)
		joins = append(joins,
			`LEFT JOIN secret_application_owner sao ON sr.secret_id = sao.secret_id`,
			`LEFT JOIN application ON application.uuid = sao.application_uuid`,
			`LEFT JOIN secret_unit_owner suo ON sr.secret_id = suo.secret_id`,
			`LEFT JOIN unit ON unit.uuid = suo.unit_uuid`,
		)
		conditions = append(conditions, `AND (
    sao.application_uuid IS NOT NULL AND application.name IN ($ApplicationOwners[:])
    OR suo.unit_uuid IS NOT NULL AND unit.unit_id IN ($UnitOwners[:])
)`)
	} else if len(appOwners) > 0 {
		queryParams = append(queryParams, appOwners)
		joins = append(joins,
			`LEFT JOIN secret_application_owner sao ON sr.secret_id = sao.secret_id`,
			`LEFT JOIN application ON application.uuid = sao.application_uuid`,
		)
		conditions = append(conditions, "AND sao.application_uuid IS NOT NULL AND application.name IN ($ApplicationOwners[:])")
	} else if len(unitOwners) > 0 {
		queryParams = append(queryParams, unitOwners)
		joins = append(joins,
			`LEFT JOIN secret_unit_owner suo ON sr.secret_id = suo.secret_id`,
			`LEFT JOIN unit ON unit.uuid = suo.unit_uuid`,
		)
		conditions = append(conditions, "AND suo.unit_uuid IS NOT NULL AND unit.unit_id IN ($UnitOwners[:])")
	}
	if len(joins) > 0 {
		q += fmt.Sprintf("\n%s", strings.Join(joins, "\n"))
	}
	if len(conditions) > 0 {
		q += fmt.Sprintf("\nWHERE %s", strings.Join(conditions, "\n"))
	}
	st.logger.Tracef(
		"revisionUUIDs %+v, appOwners: %+v, unitOwners: %+v, query: \n%s",
		revUUIDs, appOwners, unitOwners, q,
	)
	stmt, err := st.Prepare(q, append([]any{outputType}, queryParams...)...)
	if err != nil {
		return errors.Trace(err)
	}
	err = runner.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, queryParams...).GetAll(result)
		if errors.Is(err, sqlair.ErrNoRows) {
			// It's ok, the revisions probably have already been pruned.
			return nil
		}
		return errors.Trace(err)
	})
	return errors.Trace(domain.CoerceError(err))
}

type (
	revisions     []int
	revisionUUIDs []string
)

// DeleteSecret deletes the specified secret revisions.
// If revisions is nil or the last remaining revisions are removed.
func (st State) DeleteSecret(ctx context.Context, uri *coresecrets.URI, revs []int) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	// First delete the specified revisions.

	selectRevisionParams := []any{secretID{
		ID: uri.ID,
	}}

	var revFilter string
	if len(revs) > 0 {
		revFilter = "\nAND revision IN ($revisions[:])"
		selectRevisionParams = append(selectRevisionParams, revisions(revs))
	}

	selectRevsToDelete := fmt.Sprintf(`
SELECT uuid AS &revisionUUID.uuid
FROM   secret_revision
WHERE  secret_id = $secretID.id%s
`, revFilter)
	selectRevisionStmt, err := st.Prepare(selectRevsToDelete, append([]any{revisionUUID{}}, selectRevisionParams...)...)
	if err != nil {
		return errors.Trace(err)
	}

	countRevisions := `SELECT count(*) AS &M.count FROM secret_revision WHERE secret_id = $secretID.id`
	countRevisionsStmt, err := st.Prepare(countRevisions, secretID{}, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}

	deleteRevisionExpire := `
DELETE FROM secret_revision_expire WHERE revision_uuid IN ($revisionUUIDs[:])`

	deleteRevisionContent := `
DELETE FROM secret_content WHERE revision_uuid IN ($revisionUUIDs[:])`

	deleteRevisionValueRef := `
DELETE FROM secret_value_ref WHERE revision_uuid IN ($revisionUUIDs[:])`

	deleteRevisionObsolete := `
DELETE FROM secret_revision_obsolete WHERE revision_uuid IN ($revisionUUIDs[:])`

	deleteRevision := `
DELETE FROM secret_revision WHERE uuid IN ($revisionUUIDs[:])`

	deleteRevisionQueries := []string{
		deleteRevisionExpire,
		deleteRevisionContent,
		deleteRevisionValueRef,
		deleteRevisionObsolete,
		deleteRevision,
	}

	deleteRevisionStmts := make([]*sqlair.Statement, len(deleteRevisionQueries))
	for i, q := range deleteRevisionQueries {
		deleteRevisionStmts[i], err = st.Prepare(q, revisionUUIDs{})
		if err != nil {
			return errors.Trace(err)
		}
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if isLocal, err := st.checkExistsIfLocal(ctx, tx, uri); err != nil {
			return errors.Trace(err)
		} else if !isLocal {
			// Should never happen.
			return secreterrors.SecretIsNotLocal
		}

		result := []revisionUUID{}
		err := tx.Query(ctx, selectRevisionStmt, selectRevisionParams...).GetAll(&result)
		if err != nil {
			return errors.Annotatef(err, "selecting revision UUIDs to delete for secret %q", uri)
		}

		toDelete := make(revisionUUIDs, len(result))
		for i, r := range result {
			toDelete[i] = r.UUID
		}
		for _, stmt := range deleteRevisionStmts {
			err = tx.Query(ctx, stmt, toDelete).Run()
			if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
				return errors.Annotatef(err, "deleting revision info for secret %q", uri)
			}
		}

		countResult := sqlair.M{}
		err = tx.Query(ctx, countRevisionsStmt, selectRevisionParams[0]).Get(&countResult)
		if err != nil {
			return errors.Annotatef(err, "counting remaining revisions for secret %q", uri)
		}
		count, _ := strconv.Atoi(fmt.Sprint(countResult["count"]))
		if count > 0 {
			return nil
		}
		// No revisions left so delete the secret.
		return st.deleteSecret(ctx, tx, uri)
	})
	return errors.Trace(domain.CoerceError(err))
}

func (st State) deleteSecret(ctx context.Context, tx *sqlair.TX, uri *coresecrets.URI) error {
	deleteSecretRotation := `
DELETE FROM secret_rotation WHERE secret_id = $secretID.id`
	deleteSecretUnitOwner := `
DELETE FROM secret_unit_owner WHERE secret_id = $secretID.id`
	deleteSecretApplicationOwner := `
DELETE FROM secret_application_owner WHERE secret_id = $secretID.id`
	deleteSecretModelOwner := `
DELETE FROM secret_model_owner WHERE secret_id = $secretID.id`
	deleteSecretUnitConsumer := `
DELETE FROM secret_unit_consumer WHERE secret_id = $secretID.id`
	deleteSecretRemoteUnitConsumer := `
DELETE FROM secret_remote_unit_consumer WHERE secret_id = $secretID.id`
	deleteSecretRef := `
DELETE FROM secret_reference WHERE secret_id = $secretID.id`
	deleteSecretPermission := `
DELETE FROM secret_permission WHERE secret_id = $secretID.id`
	deleteSecretMetadata := `
DELETE FROM secret_metadata WHERE secret_id = $secretID.id`
	deleteSecret := `
DELETE FROM secret WHERE id = $secretID.id`

	deleteSecretQueries := []string{
		deleteSecretRotation,
		deleteSecretUnitOwner,
		deleteSecretApplicationOwner,
		deleteSecretModelOwner,
		deleteSecretUnitConsumer,
		deleteSecretRemoteUnitConsumer,
		deleteSecretRef,
		deleteSecretPermission,
		deleteSecretMetadata,
		deleteSecret,
	}

	secretIDParamParam := secretID{
		ID: uri.ID,
	}

	var err error
	deleteSecretStmts := make([]*sqlair.Statement, len(deleteSecretQueries))
	for i, q := range deleteSecretQueries {
		deleteSecretStmts[i], err = st.Prepare(q, secretIDParamParam)
		if err != nil {
			return errors.Trace(err)
		}
	}

	for _, stmt := range deleteSecretStmts {
		err = tx.Query(ctx, stmt, secretIDParamParam).Run()
		if err != nil {
			return errors.Annotatef(err, "deleting info for secret %q", uri)
		}
	}
	return nil
}
