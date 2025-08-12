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

	coreapplication "github.com/juju/juju/core/application"
	coredatabase "github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	coresecrets "github.com/juju/juju/core/secrets"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	modelerrors "github.com/juju/juju/domain/model/errors"
	domainsecret "github.com/juju/juju/domain/secret"
	secreterrors "github.com/juju/juju/domain/secret/errors"
	"github.com/juju/juju/internal/errors"
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
func (st State) GetModelUUID(ctx context.Context) (coremodel.UUID, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	var modelUUID coremodel.UUID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		modelUUID, err = st.getModelUUID(ctx, tx)
		return err
	})
	return modelUUID, errors.Capture(err)
}

func (st State) getModelUUID(ctx context.Context, tx *sqlair.TX) (coremodel.UUID, error) {
	result := modelUUID{}

	getModelUUIDStmt, err := st.Prepare("SELECT &modelUUID.uuid FROM model", result)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = tx.Query(ctx, getModelUUIDStmt).Get(&result)
	if errors.Is(err, sqlair.ErrNoRows) {
		return "", modelerrors.NotFound
	}
	if err != nil {
		return "", errors.Errorf("looking up model UUID: %w", err)
	}
	return result.UUID, nil
}

// GetApplicationUUID returns the UUID of the application with the given name, returning an error satisfying
// [applicationerrors.ApplicationNotFound] if the application does not exist.
func (st State) GetApplicationUUID(ctx domain.AtomicContext, appName string) (coreapplication.ID, error) {
	app := application{Name: appName}

	selectApplicationUUIDStmt, err := st.Prepare(`
SELECT &application.uuid
FROM application
WHERE name=$application.name`, app)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, selectApplicationUUIDStmt, app).Get(&app)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("application %q not found", appName).Add(applicationerrors.ApplicationNotFound)
		}
		if err != nil {
			return errors.Errorf("looking up application UUID for %q: %w", appName, err)
		}
		return nil
	})
	return app.UUID, errors.Capture(err)
}

// GetUnitUUID returns the UUID of the unit with the given name, returning an error satisfying
// [applicationerrors.UnitNotFound] if the unit does not exist.
func (st State) GetUnitUUID(ctx domain.AtomicContext, unitName coreunit.Name) (coreunit.UUID, error) {
	var unitUUID coreunit.UUID
	err := domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		unitUUID, err = st.getUnitUUID(ctx, tx, unitName)
		return errors.Capture(err)
	})
	return unitUUID, errors.Capture(err)
}

func (st State) getUnitUUID(ctx context.Context, tx *sqlair.TX, unitName coreunit.Name) (coreunit.UUID, error) {
	u := unit{Name: unitName}

	selectUnitUUIDStmt, err := st.Prepare(`
SELECT &unit.uuid
FROM unit
WHERE name=$unit.name`, u)
	if err != nil {
		return "", errors.Capture(err)
	}
	err = tx.Query(ctx, selectUnitUUIDStmt, u).Get(&u)
	if errors.Is(err, sqlair.ErrNoRows) {
		return "", errors.Errorf("unit %q not found", unitName).Add(applicationerrors.UnitNotFound)
	}
	if err != nil {
		return "", errors.Errorf("looking up unit UUID for %q: %w", unitName, err)
	}
	return u.UUID, errors.Capture(err)
}

// CheckApplicationSecretLabelExists checks if a charm application secret with the given label already exists.
func (st State) CheckApplicationSecretLabelExists(ctx domain.AtomicContext, appUUID coreapplication.ID, label string) (bool, error) {
	if label == "" {
		return false, nil
	}

	input := secretApplicationOwner{Label: label, ApplicationUUID: appUUID.String()}
	count := count{}
	// TODO(secrets) - we check using 2 queries, but should do in DDL
	checkLabelExistsSQL := `
SELECT COUNT(*) AS &count.num
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

	checkExistsStmt, err := st.Prepare(checkLabelExistsSQL, input, count)
	if err != nil {
		return false, errors.Capture(err)
	}

	err = domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, checkExistsStmt, input).Get(&count)
		return errors.Capture(err)
	})
	if err != nil {
		return false, errors.Errorf("checking if secret owned by application %q with label %q exists: %w", appUUID, label, err)
	}
	return count.Num > 0, nil
}

// CheckUnitSecretLabelExists checks if a charm unit secret with the given label already exists.
func (st State) CheckUnitSecretLabelExists(ctx domain.AtomicContext, unitUUID coreunit.UUID, label string) (bool, error) {
	if label == "" {
		return false, nil
	}

	input := secretUnitOwner{Label: label, UnitUUID: unitUUID.String()}
	count := count{}
	// TODO(secrets) - we check using 2 queries, but should do in DDL
	checkLabelExistsSQL := `
SELECT COUNT(*) AS &count.num
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

	checkExistsStmt, err := st.Prepare(checkLabelExistsSQL, input, count)
	if err != nil {
		return false, errors.Capture(err)
	}

	err = domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, checkExistsStmt, input).Get(&count)
		return errors.Capture(err)
	})
	if err != nil {
		return false, errors.Errorf(
			"checking if secret owned by unit %q with label %q exists: %w", unitUUID, label, err)

	}
	return count.Num > 0, nil

}

// CheckUserSecretLabelExists checks if a user secret with the given label already exists.
func (st State) CheckUserSecretLabelExists(ctx domain.AtomicContext, label string) (bool, error) {
	if label == "" {
		return false, nil
	}
	input := secretOwner{Label: label}
	count := count{}
	checkLabelExistsSQL := `
SELECT COUNT(*) AS &count.num
FROM   secret_model_owner
WHERE  label = $secretOwner.label`

	checkExistsStmt, err := st.Prepare(checkLabelExistsSQL, input, count)
	if err != nil {
		return false, errors.Capture(err)
	}
	err = domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, checkExistsStmt, input).Get(&count)
		return errors.Capture(err)
	})
	if err != nil {
		return false, errors.Errorf("checking if user secret with label %q exists: %w", label, err)
	}
	return count.Num > 0, nil
}

// CreateUserSecret creates a user secret, returning an error satisfying
// [secreterrors.SecretAlreadyExists] if a user secret with the same
// label already exists.
func (st State) CreateUserSecret(
	ctx domain.AtomicContext, version int, uri *coresecrets.URI, secret domainsecret.UpsertSecretParams,
) error {
	err := domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := st.createSecret(ctx, tx, version, uri, secret); err != nil {
			return errors.Errorf("inserting secret records for secret %q: %w", uri, err)
		}

		label := ""
		if secret.Label != nil {
			label = *secret.Label
		}
		dbSecretOwner := secretModelOwner{SecretID: uri.ID, Label: label}
		if err := st.upsertSecretModelOwner(ctx, tx, dbSecretOwner); err != nil {
			return errors.Errorf("inserting user secret record for secret %q: %w", uri, err)
		}

		modelUUID, err := st.getModelUUID(ctx, tx)
		if err != nil {
			return errors.Errorf("cannot get current model UUID for secret %q: %w", uri, err)
		}

		if err := st.grantSecretOwnerManage(ctx, tx, uri, modelUUID.String(), domainsecret.SubjectModel); err != nil {
			return errors.Errorf("granting owner manage access for secret %q: %w", uri, err)
		}
		return nil
	})
	return errors.Capture(err)
}

// CreateCharmApplicationSecret creates a secret onwed by the specified
// application, returning an error satisfying [secreterrors.SecretAlreadyExists]
// if a secretowned by the same application with the same label already exists.
// It also returns an error satisfying [applicationerrors.ApplicationNotFound]
// ifthe application does not exist.
func (st State) CreateCharmApplicationSecret(
	ctx domain.AtomicContext, version int, uri *coresecrets.URI, appUUID coreapplication.ID, secret domainsecret.UpsertSecretParams,
) error {
	if secret.AutoPrune != nil && *secret.AutoPrune {
		return secreterrors.AutoPruneNotSupported
	}

	label := ""
	if secret.Label != nil {
		label = *secret.Label
	}
	err := domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		dbSecretOwner := secretApplicationOwner{
			SecretID:        uri.ID,
			Label:           label,
			ApplicationUUID: appUUID.String(),
		}

		if err := st.createSecret(ctx, tx, version, uri, secret); err != nil {
			return errors.Errorf("inserting secret records for secret %q: %w", uri, err)
		}

		if err := st.upsertSecretApplicationOwner(ctx, tx, dbSecretOwner); err != nil {
			return errors.Errorf("inserting application secret record for secret %q: %w", uri, err)
		}

		if err := st.grantSecretOwnerManage(
			ctx, tx, uri, dbSecretOwner.ApplicationUUID, domainsecret.SubjectApplication,
		); err != nil {
			return errors.Errorf("granting owner manage access for secret %q: %w", uri, err)
		}
		return nil
	})
	return errors.Capture(err)
}

// CreateCharmUnitSecret creates a secret onwed by the specified unit,
// returning an error satisfying [secreterrors.SecretAlreadyExists] if a secret
// owned by the same unit with the same label already exists.
// It also returns an error satisfying [applicationerrors.UnitNotFound] if
// the unit does not exist.
func (st State) CreateCharmUnitSecret(
	ctx domain.AtomicContext, version int, uri *coresecrets.URI, unitUUID coreunit.UUID, secret domainsecret.UpsertSecretParams,
) error {
	if secret.AutoPrune != nil && *secret.AutoPrune {
		return secreterrors.AutoPruneNotSupported
	}

	label := ""
	if secret.Label != nil {
		label = *secret.Label
	}
	err := domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		dbSecretOwner := secretUnitOwner{
			SecretID: uri.ID,
			Label:    label,
			UnitUUID: unitUUID.String(),
		}
		if err := st.createSecret(ctx, tx, version, uri, secret); err != nil {
			return errors.Errorf("inserting secret records for secret %q: %w", uri, err)
		}

		if err := st.upsertSecretUnitOwner(ctx, tx, dbSecretOwner); err != nil {
			return errors.Errorf("inserting unit secret record for secret %q: %w", uri, err)
		}

		if err := st.grantSecretOwnerManage(ctx, tx, uri, dbSecretOwner.UnitUUID, domainsecret.SubjectUnit); err != nil {
			return errors.Errorf("granting owner manage access for secret %q: %w", uri, err)
		}
		return nil
	})
	return errors.Capture(err)
}

// UpdateSecret creates a secret with the specified parameters, returning an
// error satisfying [secreterrors.SecretNotFound] if the secret does not exist.
// It also returns an error satisfying [secreterrors.SecretLabelAlreadyExists]
// if the secret owner already has a secret with the same label.
func (st State) UpdateSecret(
	ctx domain.AtomicContext, uri *coresecrets.URI, secret domainsecret.UpsertSecretParams,
) error {
	if !secret.HasUpdate() {
		return errors.New("must specify a new value or metadata to update a secret")
	}

	err := domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := st.updateSecret(ctx, tx, uri, secret)
		if err != nil {
			return errors.Errorf("updating secret records for secret %q: %w", uri, err)
		}
		return nil
	})
	return errors.Capture(err)
}

// createSecret creates the records needed to store secret data,
// excluding secret owner records.
func (st State) createSecret(
	ctx context.Context, tx *sqlair.TX, version int, uri *coresecrets.URI,
	secret domainsecret.UpsertSecretParams,
) error {
	if len(secret.Data) == 0 && secret.ValueRef == nil {
		return errors.Errorf("cannot create a secret %q without content", uri)
	}
	if secret.RevisionID == nil {
		return errors.Errorf("revision ID must be provided")
	}

	insertQuery := `
INSERT INTO secret (id)
VALUES ($secretID.id)`

	insertStmt, err := st.Prepare(insertQuery, secretID{})
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, insertStmt, secretID{ID: uri.ID}).Run()
	if err != nil {
		return errors.Capture(err)
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
		return errors.Errorf("creating user secret %q: %w", uri, err)
	}

	dbRevision := &secretRevision{
		ID:         *secret.RevisionID,
		SecretID:   uri.ID,
		Revision:   1,
		CreateTime: now,
	}

	if err := st.upsertSecretRevision(ctx, tx, dbRevision); err != nil {
		return errors.Errorf("inserting revision for secret %q: %w", uri, err)
	}

	if secret.ExpireTime != nil {
		if err := st.upsertSecretRevisionExpiry(ctx, tx, dbRevision.ID, secret.ExpireTime); err != nil {
			return errors.Errorf("inserting revision expiry for secret %q: %w", uri, err)
		}
	}

	if secret.NextRotateTime != nil {
		if err := st.upsertSecretNextRotateTime(ctx, tx, uri, *secret.NextRotateTime); err != nil {
			return errors.Errorf("inserting next rotate time for secret %q: %w", uri, err)
		}
	}

	if len(secret.Data) > 0 {
		if err := st.updateSecretContent(ctx, tx, dbRevision.ID, secret.Data); err != nil {
			return errors.Errorf("updating content for secret %q: %w", uri, err)
		}
	}

	if secret.ValueRef != nil {
		if err := st.upsertSecretValueRef(ctx, tx, dbRevision.ID, secret.ValueRef); err != nil {
			return errors.Errorf("updating backend value reference for secret %q: %w", uri, err)
		}
	}
	return nil
}

// GetSecretOwner returns the owner of the secret with the given URI, returning an error satisfying
// [secreterrors.SecretNotFound] if the secret does not exist.
func (st State) GetSecretOwner(ctx domain.AtomicContext, uri *coresecrets.URI) (domainsecret.Owner, error) {
	input := secretID{ID: uri.ID}
	stmt, err := st.Prepare(`
SELECT
       (so.owner_kind,
        so.owner_id) AS (&secretOwner.*)
FROM   secret_metadata sm
LEFT JOIN (
    SELECT $ownerKind.model_owner_kind AS owner_kind, '' AS owner_id,  secret_id
    FROM   secret_model_owner so
    UNION
    SELECT $ownerKind.application_owner_kind AS owner_kind, application.uuid AS owner_id,  secret_id
    FROM   secret_application_owner so
    JOIN   application
    WHERE  application.uuid = so.application_uuid
    UNION
    SELECT $ownerKind.unit_owner_kind AS owner_kind, unit_uuid AS owner_id,  secret_id
    FROM   secret_unit_owner so
    JOIN   unit
    WHERE  unit.uuid = so.unit_uuid
) so ON so.secret_id = sm.secret_id
WHERE  sm.secret_id = $secretID.id
`, input, secretOwner{}, ownerKindParam)
	if err != nil {
		return domainsecret.Owner{}, errors.Capture(err)
	}

	var result []secretOwner
	err = domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, input, ownerKindParam).GetAll(&result)
		if errors.Is(err, sqlair.ErrNoRows) || len(result) == 0 {
			return errors.Errorf("secret %q not found", uri).Add(secreterrors.SecretNotFound)
		}
		return errors.Capture(err)
	})
	if err != nil {
		return domainsecret.Owner{}, errors.Capture(err)
	}
	owner := result[0]
	return domainsecret.Owner{
		UUID: owner.OwnerID,
		Kind: coresecrets.OwnerKind(owner.OwnerKind),
	}, nil
}

// createSecret creates the records needed to store secret data,
// excluding secret owner records.
func (st State) updateSecret(
	ctx context.Context, tx *sqlair.TX, uri *coresecrets.URI, secret domainsecret.UpsertSecretParams,
) error {
	// We need the latest revision so far, plus owner info for the secret,
	// so we may as well also include existing metadata as well so simplify
	// the update statement needed.
	existingSecretQuery := `
SELECT
       sm.secret_id AS &secretInfo.secret_id,
       version AS &secretInfo.version,
       description AS &secretInfo.description,
       auto_prune AS &secretInfo.auto_prune,
       rp.policy AS &secretInfo.policy,
       MAX(sr.revision) AS &secretInfo.latest_revision,
       sm.latest_revision_checksum AS &secretInfo.latest_revision_checksum,
       sr.uuid AS &secretInfo.latest_revision_uuid,
       (so.owner_kind,
        so.owner_id,
        so.label) AS (&secretOwner.*)
FROM   secret_metadata sm
       JOIN secret_revision sr ON sr.secret_id = sm.secret_id
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
WHERE  sm.secret_id = $secretID.id
GROUP BY sm.secret_id`

	existingSecretStmt, err := st.Prepare(existingSecretQuery, secretID{}, secretInfo{}, secretOwner{}, ownerKindParam)
	if err != nil {
		return errors.Capture(err)
	}

	var (
		dbSecrets      secretInfos
		dbsecretOwners []secretOwner
	)
	secretIDParam := secretID{ID: uri.ID}
	err = tx.Query(ctx, existingSecretStmt, secretIDParam, ownerKindParam).GetAll(&dbSecrets, &dbsecretOwners)
	if errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("secret %q not found", uri).Add(secreterrors.SecretNotFound)
	}
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Capture(err)
	}

	existingResult, err := dbSecrets.toSecretMetadata(dbsecretOwners)
	if err != nil {
		return errors.Capture(err)
	}
	existing := existingResult[0]
	latestRevisionUUID := dbSecrets[0].LatestRevisionUUID

	now := time.Now().UTC()
	dbSecret := secretMetadata{
		ID:             dbSecrets[0].ID,
		Version:        dbSecrets[0].Version,
		Description:    dbSecrets[0].Description,
		AutoPrune:      dbSecrets[0].AutoPrune,
		RotatePolicyID: int(domainsecret.MarshallRotatePolicy(&existing.RotatePolicy)),
		UpdateTime:     now,
	}
	dbSecret.UpdateTime = now
	updateSecretMetadataFromParams(secret, &dbSecret)
	if err := st.upsertSecret(ctx, tx, dbSecret); err != nil {
		return errors.Errorf("updating secret %q: %w", uri, err)
	}

	if secret.Label != nil {
		if err := st.upsertSecretLabel(ctx, tx, existing.URI, *secret.Label, existing.Owner); err != nil {
			return errors.Errorf("updating label for secret %q: %w", uri, err)
		}
	}

	// Will secret rotate? If not, delete next rotation row.
	if secret.RotatePolicy != nil && *secret.RotatePolicy == domainsecret.RotateNever {
		deleteNextRotate := "DELETE FROM secret_rotation WHERE secret_id=$secretID.id"
		deleteNextRotateStmt, err := st.Prepare(deleteNextRotate, secretID{})
		if err != nil {
			return errors.Capture(err)
		}
		err = tx.Query(ctx, deleteNextRotateStmt, secretIDParam).Run()
		if err != nil {
			return errors.Errorf("deleting next rotate record for secret %q: %w", uri, err)
		}
	}
	if secret.NextRotateTime != nil {
		if err := st.upsertSecretNextRotateTime(ctx, tx, uri, *secret.NextRotateTime); err != nil {
			return errors.Errorf("updating next rotate time for secret %q: %w", uri, err)
		}
	}

	var dbRevision *secretRevision
	shouldCreateNewRevision := (len(secret.Data) > 0 || secret.ValueRef != nil) && (secret.Checksum != existing.LatestRevisionChecksum ||
		// migrated charm-owned secrets from old models.
		secret.Checksum == "" && existing.LatestRevisionChecksum == "")
	if shouldCreateNewRevision {
		if secret.RevisionID == nil {
			return errors.Errorf("revision ID must be provided")
		}
		latestRevisionUUID = *secret.RevisionID
		nextRevision := existing.LatestRevision + 1
		dbRevision = &secretRevision{
			ID:         *secret.RevisionID,
			SecretID:   uri.ID,
			Revision:   nextRevision,
			CreateTime: now,
		}
	}
	if dbRevision != nil {
		if err := st.upsertSecretRevision(ctx, tx, dbRevision); err != nil {
			return errors.Errorf("inserting revision for secret %q: %w", uri, err)
		}
	}
	if secret.ExpireTime != nil {
		if err := st.upsertSecretRevisionExpiry(ctx, tx, latestRevisionUUID, secret.ExpireTime); err != nil {
			return errors.Errorf("inserting revision expiry for secret %q: %w", uri, err)
		}

	}

	if len(secret.Data) > 0 && shouldCreateNewRevision {
		if err := st.updateSecretContent(ctx, tx, dbRevision.ID, secret.Data); err != nil {
			return errors.Errorf("updating content for secret %q: %w", uri, err)
		}
	}

	if secret.ValueRef != nil && shouldCreateNewRevision {
		if err := st.upsertSecretValueRef(ctx, tx, dbRevision.ID, secret.ValueRef); err != nil {
			return errors.Errorf("updating backend value reference for secret %q: %w", uri, err)
		}
	}

	if err := st.markObsoleteRevisions(ctx, tx, uri); err != nil {
		return errors.Errorf("marking obsolete revisions for secret %q: %w", uri, err)
	}
	return nil
}

// markObsoleteRevisions obsoletes the revisions and sets the pending_delete
// to true in the secret_revision table for the specified secret if the
// revision is not the latest revision and there are no consumers for the
// revision.
func (st State) markObsoleteRevisions(ctx context.Context, tx *sqlair.TX, uri *coresecrets.URI) error {
	query, err := st.Prepare(`
SELECT sr.uuid AS &secretRevision.uuid
FROM   secret_revision sr
       LEFT JOIN (
           -- revisions that have local consumers.
           SELECT DISTINCT current_revision AS revision FROM secret_unit_consumer suc
           WHERE  suc.secret_id = $secretRef.secret_id
           UNION
           -- revisions that have remote consumers.
           SELECT DISTINCT current_revision AS revision FROM secret_remote_unit_consumer suc
           WHERE  suc.secret_id = $secretRef.secret_id
           UNION
           -- the latest revision.
           SELECT MAX(revision) FROM secret_revision rev
           WHERE  rev.secret_id = $secretRef.secret_id
       ) in_use ON sr.revision = in_use.revision
WHERE sr.secret_id = $secretRef.secret_id
AND (in_use.revision IS NULL OR in_use.revision = 0);
`, secretRef{}, secretRevision{})
	if err != nil {
		return errors.Capture(err)
	}

	stmt, err := st.Prepare(`
INSERT INTO secret_revision_obsolete (*)
VALUES ($secretRevisionObsolete.*)
ON CONFLICT(revision_uuid) DO UPDATE SET
    obsolete=excluded.obsolete,
    pending_delete=excluded.pending_delete`, secretRevisionObsolete{})
	if err != nil {
		return errors.Capture(err)
	}

	var revisionUUIIDs secretRevisions
	err = tx.Query(ctx, query, secretRef{ID: uri.ID}).GetAll(&revisionUUIIDs)
	if errors.Is(err, sqlair.ErrNoRows) {
		// No obsolete revisions to mark.
		return nil
	}
	if err != nil {
		return errors.Capture(err)
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
			return errors.Capture(err)
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
			return errors.Errorf("updating model secret record for secret %q: %w", uri, err)
		}
	case coresecrets.ApplicationOwner:
		dbSecretOwner := secretApplicationOwner{
			SecretID: uri.ID,
			// Query selects the application uuid as owner id.
			ApplicationUUID: owner.ID,
			Label:           label,
		}
		if err := st.upsertSecretApplicationOwner(ctx, tx, dbSecretOwner); err != nil {
			return errors.Errorf("updating application secret record for secret %q: %w", uri, err)
		}
	case coresecrets.UnitOwner:
		dbSecretOwner := secretUnitOwner{
			SecretID: uri.ID,
			// Query selects the unit uuid as owner id.
			UnitUUID: owner.ID,
			Label:    label,
		}
		if err := st.upsertSecretUnitOwner(ctx, tx, dbSecretOwner); err != nil {
			return errors.Errorf("updating unit secret record for secret %q: %w", uri, err)
		}
	}
	return nil
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
	md.LatestRevisionChecksum = p.Checksum
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
    latest_revision_checksum=excluded.latest_revision_checksum,
    update_time=excluded.update_time
`

	insertMetadataStmt, err := st.Prepare(insertMetadataQuery, secretMetadata{})
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, insertMetadataStmt, dbSecret).Run()
	if err != nil {
		return errors.Capture(err)
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
		return errors.Capture(err)
	}

	err = tx.Query(ctx, insertStmt, owner).Run()
	if err != nil {
		return errors.Capture(err)
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
		return errors.Capture(err)
	}

	err = tx.Query(ctx, insertStmt, owner).Run()
	if err != nil {
		return errors.Capture(err)
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
		return errors.Capture(err)
	}

	err = tx.Query(ctx, insertStmt, owner).Run()
	if err != nil {
		return errors.Capture(err)
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
		return errors.Capture(err)
	}

	err = tx.Query(ctx, insertStmt, rotate).Run()
	if err != nil {
		return errors.Capture(err)
	}
	return nil
}

func (st State) upsertSecretRevision(
	ctx context.Context, tx *sqlair.TX, dbRevision *secretRevision,
) error {
	insertQuery := `
INSERT INTO secret_revision (*)
VALUES ($secretRevision.*)`

	insertStmt, err := st.Prepare(insertQuery, secretRevision{})
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, insertStmt, dbRevision).Run()
	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

func (st State) upsertSecretRevisionExpiry(
	ctx context.Context, tx *sqlair.TX, revisionUUID string, expireTime *time.Time,
) error {
	if expireTime == nil {
		return nil
	}

	insertExpireTimeQuery := `
INSERT INTO secret_revision_expire (*)
VALUES ($secretRevisionExpire.*)
ON CONFLICT(revision_uuid) DO UPDATE SET
    expire_time=excluded.expire_time`

	expire := secretRevisionExpire{RevisionUUID: revisionUUID, ExpireTime: expireTime.UTC()}
	insertExpireTimeStmt, err := st.Prepare(insertExpireTimeQuery, expire)
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, insertExpireTimeStmt, expire).Run()
	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

func (st State) upsertSecretValueRef(
	ctx context.Context, tx *sqlair.TX, revisionUUID string, valueRef *coresecrets.ValueRef,
) error {
	upsertQuery := `
INSERT INTO secret_value_ref (*)
VALUES ($secretValueRef.*)
ON CONFLICT(revision_uuid) DO UPDATE SET
    backend_uuid=excluded.backend_uuid,
    revision_id=excluded.revision_id`

	upsertStmt, err := st.Prepare(upsertQuery, secretValueRef{})
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, upsertStmt, secretValueRef{
		RevisionUUID: revisionUUID,
		BackendUUID:  valueRef.BackendID,
		RevisionID:   valueRef.RevisionID,
	}).Run()
	if err != nil {
		return errors.Capture(err)
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
		return errors.Capture(err)
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
		return errors.Capture(err)
	}

	var keys keysToKeep
	for k := range content {
		keys = append(keys, k)
	}
	if err := tx.Query(ctx, deleteStmt, revisionUUID{UUID: revUUID}, keys).Run(); err != nil {
		return errors.Capture(err)
	}
	for key, value := range content {
		if err := tx.Query(ctx, insertStmt, secretContent{
			RevisionUUID: revUUID,
			Name:         key,
			Content:      value,
		}).Run(); err != nil {
			return errors.Capture(err)
		}
	}
	return nil
}

// InitialWatchStatementForOwnedSecrets returns the table name and query to use
// for watching changes of secrets owned by the specified apps and/or units.
func (st State) InitialWatchStatementForOwnedSecrets(
	appOwners domainsecret.ApplicationOwners, unitOwners domainsecret.UnitOwners,
) (string, eventsource.NamespaceQuery) {
	queryFunc := func(ctx context.Context, runner coredatabase.TxnRunner) ([]string, error) {
		var dbSecrets secretIDs
		err := runner.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) (err error) {
			dbSecrets, err = st.getSecretsForOwners(ctx, tx, appOwners, unitOwners)
			return errors.Capture(err)
		})
		if err != nil {
			return nil, errors.Capture(err)
		}
		var ids []string
		for _, secret := range dbSecrets {
			ids = append(ids, secret.ID)
		}
		return ids, nil
	}
	return "secret_metadata", queryFunc
}

// GetOwnedSecretIDs returns a slice of the secret ID owned by the specified apps and/or units.
func (st State) GetOwnedSecretIDs(
	ctx context.Context, appOwners domainsecret.ApplicationOwners, unitOwners domainsecret.UnitOwners,
) ([]string, error) {
	if len(appOwners) == 0 && len(unitOwners) == 0 {
		return nil, errors.New("must supply at least one app owner or unit owner")
	}

	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var dbSecrets secretIDs
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		dbSecrets, err = st.getSecretsForOwners(ctx, tx, appOwners, unitOwners)
		return errors.Capture(err)
	}); err != nil {
		return nil, errors.Errorf("getting owned secret IDs: %w", err)
	}
	var result []string
	for _, secret := range dbSecrets {
		result = append(result, secret.ID)
	}
	return result, nil
}

func (st State) getSecretsForOwners(
	ctx context.Context, tx *sqlair.TX, appOwners domainsecret.ApplicationOwners, unitOwners domainsecret.UnitOwners,
) (secretIDs, error) {
	if len(appOwners) == 0 && len(unitOwners) == 0 {
		return nil, errors.New("must supply at least one app owner or unit owner")
	}

	query := `
SELECT sm.secret_id AS &secretID.id
FROM   secret_metadata sm
       LEFT JOIN secret_application_owner sao ON sao.secret_id = sm.secret_id
       LEFT JOIN application a ON a.uuid = sao.application_uuid
       LEFT JOIN secret_unit_owner suo ON suo.secret_id = sm.secret_id
       LEFT JOIN unit u ON u.uuid = suo.unit_uuid
WHERE  (sao.application_uuid <> "" OR suo.unit_uuid <> "")
AND    (a.name IN ($ApplicationOwners[:]) OR u.name IN ($UnitOwners[:]))
`

	queryTypes := []any{
		secretID{},
		domainsecret.ApplicationOwners{},
		domainsecret.UnitOwners{},
	}

	queryStmt, err := st.Prepare(query, queryTypes...)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var dbSecrets secretIDs
	err = tx.Query(ctx, queryStmt, appOwners, unitOwners).GetAll(&dbSecrets)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Capture(err)
	}
	return dbSecrets, nil
}

// ListSecrets returns the secrets matching the specified criteria.
// If all terms are empty, then all secrets are returned.
func (st State) ListSecrets(ctx context.Context, uri *coresecrets.URI,
	revision *int,
	// TODO(secrets) - use all filter terms
	labels domainsecret.Labels,
) ([]*coresecrets.SecretMetadata, [][]*coresecrets.SecretRevisionMetadata, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, nil, errors.Capture(err)
	}

	var revisionNotFoundErr error
	if revision != nil {
		revisionNotFoundErr = errors.Errorf(
			"secret revision %d for %s not found", *revision, uri).Add(secreterrors.SecretRevisionNotFound)
	}

	var (
		secrets        []*coresecrets.SecretMetadata
		revisionResult [][]*coresecrets.SecretRevisionMetadata
	)
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		secrets, err = st.listSecretsAnyOwner(ctx, tx, uri)
		if err != nil {
			return errors.Errorf("querying secrets: %w", err)
		}
		revisionResult = make([][]*coresecrets.SecretRevisionMetadata, len(secrets))
		for i, secret := range secrets {
			secretRevisions, err := st.listSecretRevisions(ctx, tx, secret.URI, revision)
			if err != nil {
				return errors.Errorf("querying secret revisions for %q: %w", secret.URI.ID, err)
			}
			revisionResult[i] = secretRevisions
			if revision != nil && len(secretRevisions) == 0 {
				return revisionNotFoundErr
			}
		}
		return nil
	}); err != nil {
		return nil, nil, errors.Capture(err)
	}
	if revision != nil && len(secrets) == 0 {
		return nil, nil, revisionNotFoundErr
	}

	return secrets, revisionResult, nil
}

// GetSecret returns the secret with the given URI, returning an error satisfying [secreterrors.SecretNotFound]
// if the secret does not exist.
func (st State) GetSecret(ctx context.Context, uri *coresecrets.URI) (*coresecrets.SecretMetadata, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var secrets []*coresecrets.SecretMetadata
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		secrets, err = st.listSecretsAnyOwner(ctx, tx, uri)
		if err != nil {
			return errors.Errorf("querying secret for %q: %w", uri.ID, err)
		}
		return nil
	}); err != nil {
		return nil, errors.Capture(err)
	}

	if len(secrets) == 0 {
		return nil, errors.Errorf("secret %q not found", uri).Add(secreterrors.SecretNotFound)
	}
	return secrets[0], nil
}

// GetLatestRevision returns the latest revision number for the specified secret,
// returning an error satisfying [secreterrors.SecretNotFound] if the secret does not exist.
func (st State) GetLatestRevision(ctx context.Context, uri *coresecrets.URI) (int, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return 0, errors.Capture(err)
	}
	query := `
SELECT MAX(sr.revision) AS &secretInfo.latest_revision
FROM   secret_revision sr
WHERE  sr.secret_id = $secretInfo.secret_id
`
	info := secretInfo{
		ID: uri.ID,
	}
	stmt, err := st.Prepare(query, info)
	if err != nil {
		return 0, errors.Capture(err)
	}
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, info).Get(&info)
		if err != nil {
			return errors.Capture(err)
		}
		if info.LatestRevision == 0 {
			return errors.Errorf("secret %q not found", uri).Add(secreterrors.SecretNotFound)
		}
		return nil
	}); err != nil {
		return 0, errors.Capture(err)
	}
	return info.LatestRevision, nil
}

// GetRotationExpiryInfo returns the rotation expiry information for the specified secret.
func (st State) GetRotationExpiryInfo(ctx context.Context, uri *coresecrets.URI) (*domainsecret.RotationExpiryInfo, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	input := secretID{ID: uri.ID}
	result := secretInfo{}
	stmt, err := st.Prepare(`
SELECT   sp.policy AS &secretInfo.policy,
         sro.next_rotation_time AS &secretInfo.next_rotation_time,
         sre.expire_time AS &secretInfo.latest_expire_time,
         MAX(sr.revision) AS &secretInfo.latest_revision
FROM     secret_metadata sm
         JOIN secret_revision sr ON sm.secret_id = sr.secret_id
         JOIN secret_rotate_policy sp ON sp.id = sm.rotate_policy_id
         LEFT JOIN secret_rotation sro ON sro.secret_id = sm.secret_id
         LEFT JOIN secret_revision_expire sre ON sre.revision_uuid = sr.uuid
WHERE    sm.secret_id = $secretID.id
GROUP BY sr.secret_id`, input, result)

	if err != nil {
		return nil, errors.Capture(err)
	}

	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, input).Get(&result)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("secret %q not found", uri).Add(secreterrors.SecretNotFound)
		}
		return errors.Capture(err)
	}); err != nil {
		return nil, errors.Capture(err)
	}
	info := &domainsecret.RotationExpiryInfo{
		RotatePolicy:   coresecrets.RotatePolicy(result.RotatePolicy),
		LatestRevision: result.LatestRevision,
	}
	if !result.NextRotateTime.IsZero() {
		info.NextRotateTime = &result.NextRotateTime
	}
	if !result.LatestExpireTime.IsZero() {
		info.LatestExpireTime = &result.LatestExpireTime
	}
	return info, nil
}

// GetRotatePolicy returns the rotate policy for the specified secret.
func (st State) GetRotatePolicy(ctx context.Context, uri *coresecrets.URI) (coresecrets.RotatePolicy, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return coresecrets.RotateNever, errors.Capture(err)
	}
	stmt, err := st.Prepare(`
SELECT srp.policy AS &secretInfo.policy
FROM   secret_metadata sm
       JOIN secret_rotate_policy srp ON srp.id = sm.rotate_policy_id
WHERE  sm.secret_id = $secretID.id`, secretID{}, secretInfo{})
	if err != nil {
		return coresecrets.RotateNever, errors.Capture(err)
	}

	var info secretInfo
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, secretID{ID: uri.ID}).Get(&info)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("rotate policy for %q not found", uri).Add(secreterrors.SecretNotFound)
		}
		return errors.Capture(err)
	}); err != nil {
		return coresecrets.RotateNever, errors.Capture(err)
	}
	return coresecrets.RotatePolicy(info.RotatePolicy), nil
}

func (st State) listSecretsAnyOwner(
	ctx context.Context, tx *sqlair.TX, uri *coresecrets.URI,
) ([]*coresecrets.SecretMetadata, error) {

	query := `
SELECT sm.secret_id AS &secretInfo.secret_id,
       sm.version AS &secretInfo.version,
       sm.description AS &secretInfo.description,
       sm.auto_prune AS &secretInfo.auto_prune,
       sm.latest_revision_checksum AS &secretInfo.latest_revision_checksum,
       sm.create_time AS &secretInfo.create_time,
       sm.update_time AS &secretInfo.update_time,
       rp.policy AS &secretInfo.policy,
       sro.next_rotation_time AS &secretInfo.next_rotation_time,
       sre.expire_time AS &secretInfo.latest_expire_time,
       MAX(sr.revision) AS &secretInfo.latest_revision,
       (so.owner_kind,
       so.owner_id,
       so.label) AS (&secretOwner.*)
FROM   secret_metadata sm
       JOIN secret_revision sr ON sm.secret_id = sr.secret_id
       LEFT JOIN secret_revision_expire sre ON sre.revision_uuid = sr.uuid
       LEFT JOIN secret_rotate_policy rp ON rp.id = sm.rotate_policy_id
       LEFT JOIN secret_rotation sro ON sro.secret_id = sm.secret_id
       LEFT JOIN (
          SELECT $ownerKind.model_owner_kind AS owner_kind, (SELECT uuid FROM model) AS owner_id, label, secret_id
          FROM   secret_model_owner so
          UNION
          SELECT $ownerKind.application_owner_kind AS owner_kind, application.name AS owner_id, label, secret_id
          FROM   secret_application_owner so
          JOIN   application
          WHERE  application.uuid = so.application_uuid
          UNION
          SELECT $ownerKind.unit_owner_kind AS owner_kind, unit.name AS owner_id, label, secret_id
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
	query += "\nGROUP BY sm.secret_id"
	queryStmt, err := st.Prepare(query, queryTypes...)
	if err != nil {
		st.logger.Tracef(ctx, "failed to prepare err: %v, query: \n%s", err, query)
		return nil, errors.Capture(err)
	}

	var (
		dbSecrets      secretInfos
		dbsecretOwners []secretOwner
	)
	err = tx.Query(ctx, queryStmt, queryParams...).GetAll(&dbSecrets, &dbsecretOwners)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Capture(err)
	}
	return dbSecrets.toSecretMetadata(dbsecretOwners)
}

// ListCharmSecrets returns charm secrets owned by the specified applications and/or units.
// At least one owner must be specified.
func (st State) ListCharmSecrets(ctx context.Context,
	appOwners domainsecret.ApplicationOwners, unitOwners domainsecret.UnitOwners,
) ([]*coresecrets.SecretMetadata, [][]*coresecrets.SecretRevisionMetadata, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, nil, errors.Capture(err)
	}

	var (
		secrets        []*coresecrets.SecretMetadata
		revisionResult [][]*coresecrets.SecretRevisionMetadata
	)
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		secrets, err = st.listCharmSecrets(ctx, tx, appOwners, unitOwners)
		if err != nil {
			return errors.Errorf("querying charm secrets: %w", err)
		}
		revisionResult = make([][]*coresecrets.SecretRevisionMetadata, len(secrets))
		for i, secret := range secrets {
			secretRevisions, err := st.listSecretRevisions(ctx, tx, secret.URI, nil)
			if err != nil {
				return errors.Errorf("querying secret revisions for %q: %w", secret.URI.ID, err)
			}
			revisionResult[i] = secretRevisions
		}
		return nil
	}); err != nil {
		return nil, nil, errors.Capture(err)
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

	var preQueryParts []string
	appOwnerSelect := `
app_owners AS
    (SELECT $ownerKind.application_owner_kind AS owner_kind, application.name AS owner_id, label, secret_id
     FROM   secret_application_owner so
     JOIN   application ON application.uuid = so.application_uuid
     AND application.name IN ($ApplicationOwners[:]))`[1:]

	unitOwnerSelect := `
unit_owners AS
    (SELECT $ownerKind.unit_owner_kind AS owner_kind, unit.name AS owner_id, label, secret_id
     FROM   secret_unit_owner so
     JOIN   unit ON unit.uuid = so.unit_uuid
     AND unit.name IN ($UnitOwners[:]))`[1:]

	if len(appOwners) > 0 {
		preQueryParts = append(preQueryParts, appOwnerSelect)
	}
	if len(unitOwners) > 0 {
		preQueryParts = append(preQueryParts, unitOwnerSelect)
	}
	var queryParts []string
	if len(preQueryParts) > 0 {
		queryParts = append(queryParts, `WITH `+strings.Join(preQueryParts, ",\n"))
	}

	query := `
SELECT sm.secret_id AS &secretInfo.secret_id,
       sm.version AS &secretInfo.version,
       sm.description AS &secretInfo.description,
       sm.auto_prune AS &secretInfo.auto_prune,
       rp.policy AS &secretInfo.policy,
       sro.next_rotation_time AS &secretInfo.next_rotation_time,
       sre.expire_time AS &secretInfo.latest_expire_time,
       sm.latest_revision_checksum AS &secretInfo.latest_revision_checksum,
       sm.create_time AS &secretInfo.create_time,
       sm.update_time AS &secretInfo.update_time,
       MAX(sr.revision) AS &secretInfo.latest_revision,
       (so.owner_kind,
       so.owner_id,
       so.label) AS (&secretOwner.*)
FROM   secret_metadata sm
       JOIN secret_revision sr ON sr.secret_id = sm.secret_id
       LEFT JOIN secret_revision_expire sre ON sre.revision_uuid = sr.uuid
       LEFT JOIN secret_rotate_policy rp ON rp.id = sm.rotate_policy_id
       LEFT JOIN secret_rotation sro ON sro.secret_id = sm.secret_id`

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

	queryParts = append(queryParts, ownerJoin, `GROUP BY sm.secret_id`)
	queryStmt, err := st.Prepare(strings.Join(queryParts, "\n"), queryTypes...)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var (
		dbSecrets      secretInfos
		dbsecretOwners []secretOwner
	)
	err = tx.Query(ctx, queryStmt, queryParams...).GetAll(&dbSecrets, &dbsecretOwners)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Capture(err)
	}
	return dbSecrets.toSecretMetadata(dbsecretOwners)
}

// ListUserSecretsToDrain returns secret drain revision info for any user secrets.
func (st State) ListUserSecretsToDrain(ctx context.Context) ([]*coresecrets.SecretMetadataForDrain, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
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
		return nil, errors.Capture(err)
	}

	var (
		dbSecrets    secretIDs
		dbsecretRevs secretExternalRevisions
	)

	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, queryStmt).GetAll(&dbSecrets, &dbsecretRevs)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		}
		return nil
	}); err != nil {
		return nil, errors.Capture(err)
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

	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
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
     AND unit.name IN ($UnitOwners[:]))`[1:]

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
SELECT sm.secret_id AS &secretID.id,
       svr.backend_uuid AS &secretExternalRevision.backend_uuid,
       svr.revision_id AS &secretExternalRevision.revision_id,
       rev.revision AS &secretExternalRevision.revision
FROM   secret_metadata sm
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
		return nil, errors.Capture(err)
	}

	var (
		dbSecrets    secretIDs
		dbsecretRevs secretExternalRevisions
	)

	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, queryStmt, queryParams...).GetAll(&dbSecrets, &dbsecretRevs)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		}
		return nil
	}); err != nil {
		return nil, errors.Capture(err)
	}

	return dbSecrets.toSecretMetadataForDrain(dbsecretRevs)
}

// GetUserSecretURIByLabel returns the URI for the user secret with the specified label,
// or an error satisfying [secreterrors.SecretNotFound] if there's no corresponding URI.
func (st State) GetUserSecretURIByLabel(ctx context.Context, label string) (*coresecrets.URI, error) {
	if label == "" {
		return nil, errors.Errorf("empty secret label %w", coreerrors.NotValid)
	}

	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	query := `
SELECT sm.secret_id AS &secretInfo.secret_id
FROM   secret_metadata sm
JOIN   secret_model_owner mso ON sm.secret_id = mso.secret_id
WHERE  mso.label = $M.label
	`
	arg := sqlair.M{"label": label}

	queryStmt, err := st.Prepare(query, secretInfo{}, arg)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var dbSecrets secretInfos
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, queryStmt, arg).GetAll(&dbSecrets)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("querying secret URI for label %q: %w", label, err)
		}
		return nil
	}); err != nil {
		return nil, errors.Capture(err)
	}

	if len(dbSecrets) == 0 {
		return nil, errors.Errorf("secret with label %q not found", label).Add(secreterrors.SecretNotFound)
	}
	return coresecrets.ParseURI(dbSecrets[0].ID)
}

// GetURIByConsumerLabel looks up the secret URI using the label previously
// registered by the specified unit,returning an error satisfying
// [secreterrors.SecretNotFound] if there's no corresponding URI.
// If the unit does not exist, an error satisfying [applicationerrors.UnitNotFound]
// is returned.
func (st State) GetURIByConsumerLabel(ctx context.Context, label string, unitName coreunit.Name) (*coresecrets.URI, error) {
	if label == "" {
		return nil, errors.Errorf("empty secret label %w", coreerrors.NotValid)
	}

	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	query := `
SELECT secret_id AS &secretUnitConsumer.secret_id,
       source_model_uuid AS &secretUnitConsumer.source_model_uuid
FROM   secret_unit_consumer suc
WHERE  suc.label = $secretUnitConsumer.label
AND    suc.unit_uuid = $secretUnitConsumer.unit_uuid
`

	queryStmt, err := st.Prepare(query, secretUnitConsumer{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var dbConsumers []secretUnitConsumer
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		suc := secretUnitConsumer{Label: label}
		suc.UnitUUID, err = st.getUnitUUID(ctx, tx, unitName)
		if err != nil {
			return errors.Capture(err)
		}

		err := tx.Query(ctx, queryStmt, suc).GetAll(&dbConsumers)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("querying secret URI for label %q: %w", label, err)
		}
		return nil
	}); err != nil {
		return nil, errors.Capture(err)
	}

	if len(dbConsumers) == 0 {
		return nil, errors.Errorf(
			"secret with label %q for unit %q not found", label, unitName).Add(secreterrors.SecretNotFound)
	}
	uri, err := coresecrets.ParseURI(dbConsumers[0].SecretID)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return uri.WithSource(dbConsumers[0].SourceModelUUID), nil
}

func (st State) listSecretRevisions(
	ctx context.Context, tx *sqlair.TX, uri *coresecrets.URI, revision *int,
) ([]*coresecrets.SecretRevisionMetadata, error) {
	query := `
SELECT (sr.*) AS (&secretRevision.*),
       (svr.*) AS (&secretValueRef.*),
       (sre.*) AS (&secretRevisionExpire.*)
FROM   secret_revision sr
       LEFT JOIN secret_revision_expire sre ON sre.revision_uuid = sr.uuid
       LEFT JOIN secret_value_ref svr ON svr.revision_uuid = sr.uuid
WHERE  secret_id = $secretRevision.secret_id
`
	want := secretRevision{SecretID: uri.ID}
	if revision != nil {
		query = query + "\nAND revision = $secretRevision.revision"
		want.Revision = *revision
	}

	queryStmt, err := st.Prepare(query, secretRevision{}, secretRevisionExpire{}, secretValueRef{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var (
		dbSecretRevisions       secretRevisions
		dbSecretValueRefs       secretValueRefs
		dbSecretRevisionsExpire secretRevisionsExpire
	)
	err = tx.Query(ctx, queryStmt, want).GetAll(&dbSecretRevisions, &dbSecretValueRefs, &dbSecretRevisionsExpire)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Errorf("retrieving secret revisions for %q: %w", uri, err)
	}

	return dbSecretRevisions.toSecretRevisions(dbSecretValueRefs, dbSecretRevisionsExpire)
}

// GetSecretValue returns the contents - either data or value reference - of a
// given secret revision, returning an error satisfying
// [secreterrors.SecretRevisionNotFound] if the secret revision does not exist.
func (st State) GetSecretValue(
	ctx context.Context, uri *coresecrets.URI, revision int) (coresecrets.SecretData, *coresecrets.ValueRef, error,
) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, nil, errors.Capture(err)
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
		return nil, nil, errors.Capture(err)
	}

	valueRefQuery := `
SELECT (*) AS (&secretValueRef.*)
FROM   secret_value_ref val
       JOIN secret_revision rev ON val.revision_uuid = rev.uuid
WHERE  rev.secret_id = $secretRevision.secret_id
AND    rev.revision = $secretRevision.revision`

	valueRefQueryStmt, err := st.Prepare(valueRefQuery, secretValueRef{}, secretRevision{})
	if err != nil {
		return nil, nil, errors.Capture(err)
	}

	want := secretRevision{SecretID: uri.ID, Revision: revision}

	var (
		dbSecretValues    secretValues
		dbSecretValueRefs []secretValueRef
	)
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, contentQueryStmt, want).GetAll(&dbSecretValues)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("retrieving secret value for %q revision %d: %w", uri, revision, err)
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
		if err != nil {
			return errors.Errorf("retrieving secret value ref for %q revision %d: %w", uri, revision, err)
		}
		return nil
	}); err != nil {
		return nil, nil, errors.Errorf("querying secret value: %w", err)
	}

	// Compose and return any secret content from the db.
	if len(dbSecretValues) > 0 {
		content := dbSecretValues.toSecretData()
		return content, nil, nil
	}

	// Process any value reference.
	if len(dbSecretValueRefs) == 0 {
		return nil, nil, errors.Errorf(
			"secret value ref for %q revision %d not found", uri, revision).Add(secreterrors.SecretRevisionNotFound)
	}
	if len(dbSecretValueRefs) != 1 {
		return nil, nil, errors.Errorf(
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

	result := sqlair.M{}

	ref := secretRef{ID: uri.ID, SourceUUID: uri.SourceUUID}
	queryStmt, err := st.Prepare(query, ref, result)
	if err != nil {
		return false, errors.Capture(err)
	}
	err = tx.Query(ctx, queryStmt, ref).Get(&result)
	if err == nil {
		isLocal := result["is_local"]
		return isLocal == "local", nil
	}
	if errors.Is(err, sqlair.ErrNoRows) {
		return false, secreterrors.SecretNotFound
	}
	if err != nil {
		return false, errors.Errorf("looking up secret URI %q: %w", uri, err)
	}
	return false, nil
}

// GetSecretConsumer returns the secret consumer info for the specified unit
// and secret, along withthe latest revision for the secret.
// If the unit does not exist, an error satisfying [applicationerrors.UnitNotFound] is
// returned.If the secret does not exist, an error satisfying
// [secreterrors.SecretNotFound] is returned.
// If there's not currently a consumer record for the secret, the latest
// revision is still returned,along with an error satisfying
// [secreterrors.SecretConsumerNotFound].
func (st State) GetSecretConsumer(
	ctx context.Context, uri *coresecrets.URI, unitName coreunit.Name,
) (*coresecrets.SecretConsumerMetadata, int, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, 0, errors.Capture(err)
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
		return nil, 0, errors.Capture(err)
	}

	selectLatestLocalRevision := `
SELECT MAX(revision) AS &secretRef.revision
FROM   secret_revision rev
WHERE  rev.secret_id = $secretRef.secret_id`
	selectLatestLocalRevisionStmt, err := st.Prepare(selectLatestLocalRevision, secretRef{})
	if err != nil {
		return nil, 0, errors.Capture(err)
	}

	selectLatestRemoteRevision := `
SELECT latest_revision AS &secretRef.revision
FROM   secret_reference ref
WHERE  ref.secret_id = $secretRef.secret_id`
	selectLatestRemoteRevisionStmt, err := st.Prepare(selectLatestRemoteRevision, secretRef{})
	if err != nil {
		return nil, 0, errors.Capture(err)
	}

	var (
		dbSecretConsumers secretUnitConsumers
		latestRevision    int
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		isLocal, err := st.checkExistsIfLocal(ctx, tx, uri)
		if err != nil {
			return errors.Capture(err)
		}

		consumer.UnitUUID, err = st.getUnitUUID(ctx, tx, unitName)
		if err != nil {
			return errors.Capture(err)
		}
		err = tx.Query(ctx, queryStmt, consumer).GetAll(&dbSecretConsumers)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("querying secret consumers: %w", err)
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
				return errors.Errorf("looking up latest revision for %q: %w", uri.ID, err)
			}
		}
		latestRevision = latest.Revision

		return nil
	})
	if err != nil {
		return nil, 0, errors.Capture(err)
	}
	if len(dbSecretConsumers) == 0 {
		return nil, latestRevision, errors.Errorf("secret consumer for %q and unit %q %w", uri.ID, unitName, secreterrors.SecretConsumerNotFound)
	}
	consumers := dbSecretConsumers.toSecretConsumers()
	return consumers[0], latestRevision, nil
}

// SaveSecretConsumer saves the consumer metadata for the given secret and unit.
// If the unit does not exist, an error satisfying [applicationerrors.UnitNotFound] is returned.
// If the secret does not exist, an error satisfying [secreterrors.SecretNotFound] is returned.
func (st State) SaveSecretConsumer(
	ctx context.Context, uri *coresecrets.URI, unitName coreunit.Name, md *coresecrets.SecretConsumerMetadata,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	insertQuery := `
INSERT INTO secret_unit_consumer (*)
VALUES ($secretUnitConsumer.*)
ON CONFLICT(secret_id, unit_uuid) DO UPDATE SET
    label=excluded.label,
    current_revision=excluded.current_revision`

	insertStmt, err := st.Prepare(insertQuery, secretUnitConsumer{})
	if err != nil {
		return errors.Capture(err)
	}

	// We might be saving a tracked revision for a remote secret
	// before we have been notified of a revision change.
	// So we might need to insert the parent secret URI.
	secretRef := secretID{ID: uri.ID}
	insertRemoteSecretQuery := `
INSERT INTO secret (id)
VALUES ($secretID.id)
ON CONFLICT DO NOTHING`

	insertRemoteSecretStmt, err := st.Prepare(insertRemoteSecretQuery, secretRef)
	if err != nil {
		return errors.Capture(err)
	}

	remoteRef := remoteSecret{SecretID: uri.ID, LatestRevision: md.CurrentRevision}
	insertRemoteSecretReferenceQuery := `
INSERT INTO secret_reference (secret_id, latest_revision)
VALUES ($remoteSecret.secret_id, $remoteSecret.latest_revision)
ON CONFLICT DO NOTHING`

	insertRemoteSecretReferenceStmt, err := st.Prepare(insertRemoteSecretReferenceQuery, remoteRef)
	if err != nil {
		return errors.Capture(err)
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
			return errors.Capture(err)
		}

		if !isLocal {
			// Ensure a remote secret parent URI and revision is recorded.
			// This will normally be done by the watcher but it may not have fired yet.
			err = tx.Query(ctx, insertRemoteSecretStmt, secretRef).Run()
			if err != nil {
				return errors.Errorf("inserting remote secret reference for %q: %w", uri, err)
			}
			err = tx.Query(ctx, insertRemoteSecretReferenceStmt, remoteRef).Run()
			if err != nil {
				return errors.Errorf("inserting remote secret revision for %q: %w", uri, err)
			}
		}
		consumer.UnitUUID, err = st.getUnitUUID(ctx, tx, unitName)
		if err != nil {
			return errors.Capture(err)
		}

		if err := tx.Query(ctx, insertStmt, consumer).Run(); err != nil {
			return errors.Capture(err)
		}

		if err := st.markObsoleteRevisions(ctx, tx, uri); err != nil {
			return errors.Errorf("marking obsolete revisions for secret %q: %w", uri, err)
		}

		return nil
	})
	return errors.Capture(err)
}

// AllSecretConsumers loads all local secret consumers keyed by secret id.
func (st State) AllSecretConsumers(ctx context.Context) (map[string][]domainsecret.ConsumerInfo, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	query := `
SELECT suc.secret_id AS &secretUnitConsumerInfo.secret_id,
       suc.label AS &secretUnitConsumerInfo.label,
       suc.current_revision AS &secretUnitConsumerInfo.current_revision,
       u.name AS &secretUnitConsumerInfo.unit_name
FROM   secret_unit_consumer suc
       JOIN secret_metadata sm ON sm.secret_id = suc.secret_id
       JOIN unit u ON u.uuid = suc.unit_uuid
`

	queryStmt, err := st.Prepare(query, secretUnitConsumerInfo{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var dbSecretConsumers secretUnitConsumerInfos
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, queryStmt).GetAll(&dbSecretConsumers)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("querying secret consumers: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	consumers := dbSecretConsumers.toSecretConsumersBySecret()
	return consumers, nil
}

// GetSecretRemoteConsumer returns the secret consumer info from a cross model consumer
// for the specified unit and secret.
// If the secret does not exist, an error satisfying [secreterrors.SecretNotFound] is returned.
// If there's not currently a consumer record for the secret, the latest revision is still returned,
// along with an error satisfying [secreterrors.SecretConsumerNotFound].
func (st State) GetSecretRemoteConsumer(
	ctx context.Context, uri *coresecrets.URI, unitName coreunit.Name,
) (*coresecrets.SecretConsumerMetadata, int, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, 0, errors.Capture(err)
	}

	consumer := secretRemoteUnitConsumer{
		SecretID: uri.ID,
		UnitName: unitName,
	}

	query := `
SELECT suc.current_revision AS &secretRemoteUnitConsumer.current_revision
FROM   secret_remote_unit_consumer suc
WHERE  suc.secret_id = $secretRemoteUnitConsumer.secret_id
AND    suc.unit_name = $secretRemoteUnitConsumer.unit_name`

	queryStmt, err := st.Prepare(query, secretRemoteUnitConsumer{})
	if err != nil {
		return nil, 0, errors.Capture(err)
	}

	selectLatestRevision := `
SELECT MAX(revision) AS &secretInfo.latest_revision
FROM   secret_revision rev
WHERE  rev.secret_id = $secretInfo.secret_id`
	selectLatestRevisionStmt, err := st.Prepare(selectLatestRevision, secretInfo{})
	if err != nil {
		return nil, 0, errors.Capture(err)
	}

	var (
		dbSecretConsumers secretRemoteUnitConsumers
		latestRevision    int
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if isLocal, err := st.checkExistsIfLocal(ctx, tx, uri); err != nil {
			return errors.Capture(err)
		} else if !isLocal {
			// Should never happen.
			return secreterrors.SecretIsNotLocal
		}

		err = tx.Query(ctx, queryStmt, consumer).GetAll(&dbSecretConsumers)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("querying secret consumer info for secret %q and unit %q: %w", uri, unitName, err)
		}

		result := secretInfo{ID: uri.ID}
		err = tx.Query(ctx, selectLatestRevisionStmt, result).Get(&result)
		if err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return secreterrors.SecretNotFound
			} else {
				return errors.Errorf("looking up latest revision for %q: %w", uri.ID, err)
			}
		}
		latestRevision = result.LatestRevision

		return nil
	})
	if err != nil {
		return nil, 0, errors.Capture(err)
	}
	if len(dbSecretConsumers) == 0 {
		return nil, latestRevision, errors.Errorf(
			"secret consumer for %q and unit %q %w", uri.ID, unitName, secreterrors.SecretConsumerNotFound)

	}
	consumers := dbSecretConsumers.toSecretConsumers()
	return consumers[0], latestRevision, nil
}

// SaveSecretRemoteConsumer saves the consumer metadata for the given secret and unit.
// If the secret does not exist, an error satisfying [secreterrors.SecretNotFound] is returned.
func (st State) SaveSecretRemoteConsumer(
	ctx context.Context, uri *coresecrets.URI, unitName coreunit.Name, md *coresecrets.SecretConsumerMetadata,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	insertQuery := `
INSERT INTO secret_remote_unit_consumer (*)
VALUES ($secretRemoteUnitConsumer.*)
ON CONFLICT(secret_id, unit_name) DO UPDATE SET
    current_revision=excluded.current_revision`

	insertStmt, err := st.Prepare(insertQuery, secretRemoteUnitConsumer{})
	if err != nil {
		return errors.Capture(err)
	}

	consumer := secretRemoteUnitConsumer{
		SecretID:        uri.ID,
		UnitName:        unitName,
		CurrentRevision: md.CurrentRevision,
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if isLocal, err := st.checkExistsIfLocal(ctx, tx, uri); err != nil {
			return errors.Capture(err)
		} else if !isLocal {
			// Should never happen.
			return secreterrors.SecretIsNotLocal
		}
		if err := tx.Query(ctx, insertStmt, consumer).Run(); err != nil {
			return errors.Capture(err)
		}

		if err := st.markObsoleteRevisions(ctx, tx, uri); err != nil {
			return errors.Errorf("marking obsolete revisions for secret %q: %w", uri, err)
		}

		return nil
	})
	return errors.Capture(err)
}

// AllSecretRemoteConsumers loads all secret remote consumers keyed by secret id.
func (st State) AllSecretRemoteConsumers(ctx context.Context) (map[string][]domainsecret.ConsumerInfo, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	query := `
SELECT suc.secret_id AS &secretUnitConsumerInfo.secret_id,
       suc.current_revision AS &secretUnitConsumerInfo.current_revision,
       suc.unit_name AS &secretUnitConsumerInfo.unit_name
FROM   secret_remote_unit_consumer suc
`

	queryStmt, err := st.Prepare(query, secretUnitConsumerInfo{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var dbSecretConsumers secretUnitConsumerInfos
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, queryStmt).GetAll(&dbSecretConsumers)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("querying secret remote consumers: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	consumers := dbSecretConsumers.toSecretConsumersBySecret()
	return consumers, nil
}

// UpdateRemoteSecretRevision records the latest revision
// of the specified cross model secret.
func (st State) UpdateRemoteSecretRevision(ctx context.Context, uri *coresecrets.URI, latestRevision int) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	insertQuery := `
INSERT INTO secret (id)
VALUES ($secretID.id)
ON CONFLICT(id) DO NOTHING`

	insertStmt, err := st.Prepare(insertQuery, secretID{})
	if err != nil {
		return errors.Capture(err)
	}

	insertLatestQuery := `
INSERT INTO secret_reference (*)
VALUES ($remoteSecret.*)
ON CONFLICT(secret_id) DO UPDATE SET
    latest_revision=excluded.latest_revision`

	insertLatestStmt, err := st.Prepare(insertLatestQuery, remoteSecret{})
	if err != nil {
		return errors.Capture(err)
	}

	secret := remoteSecret{
		SecretID:       uri.ID,
		LatestRevision: latestRevision,
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, insertStmt, secretID{ID: uri.ID}).Run()
		if err != nil {
			return errors.Errorf("inserting URI record for cross model secret %q: %w", uri, err)
		}
		if err := tx.Query(ctx, insertLatestStmt, secret).Run(); err != nil {
			return errors.Errorf("updating latest revision %d for cross model secret %q: %w", latestRevision, uri, err)
		}
		if err := st.markObsoleteRevisions(ctx, tx, uri); err != nil {
			return errors.Errorf("marking obsolete revisions for secret %q: %w", uri, err)
		}
		return nil
	})
	return errors.Capture(err)
}

// AllRemoteSecrets returns consumer info for secrets stored in
// an external model.
func (st State) AllRemoteSecrets(ctx context.Context) ([]domainsecret.RemoteSecretInfo, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	q := `
SELECT suc.secret_id AS &secretUnitConsumerInfo.secret_id,
       suc.source_model_uuid AS &secretUnitConsumerInfo.source_model_uuid,
       suc.label AS &secretUnitConsumerInfo.label,
       suc.current_revision AS &secretUnitConsumerInfo.current_revision,
       sr.latest_revision AS &secretUnitConsumerInfo.latest_revision,
       u.name AS &secretUnitConsumerInfo.unit_name
FROM   secret_unit_consumer suc
       JOIN unit u ON u.uuid = suc.unit_uuid
       JOIN secret_reference sr ON sr.secret_id = suc.secret_id
`

	stmt, err := st.Prepare(q, secretUnitConsumerInfo{})
	if err != nil {
		return nil, errors.Capture(err)
	}
	var dbSecretConsumers secretUnitConsumerInfos
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).GetAll(&dbSecretConsumers)
		if errors.Is(err, sqlair.ErrNoRows) {
			// No secrets found.
			return nil
		}
		return errors.Capture(err)
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	secrets := dbSecretConsumers.toRemoteSecrets()
	return secrets, nil
}

// GrantAccess grants access to the secret for the specified subject with the specified scope.
// It returns an error satisfying [secreterrors.SecretNotFound] if the secret is not found.
// If an attempt is made to change an existing permission's scope or subject type, an error
// satisfying [secreterrors.InvalidSecretPermissionChange] is returned.
func (st State) GrantAccess(ctx context.Context, uri *coresecrets.URI, params domainsecret.GrantParams) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
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
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		perm := secretPermission{
			SecretID: uri.ID,
			RoleID:   params.RoleID,
		}
		if isLocal, err := st.checkExistsIfLocal(ctx, tx, uri); err != nil {
			return errors.Capture(err)
		} else if !isLocal {
			// Should never happen.
			return secreterrors.SecretIsNotLocal
		}

		// Look up the UUID of the subject.
		perm.SubjectTypeID = params.SubjectTypeID
		perm.SubjectUUID, err = st.lookupSubjectUUID(ctx, tx, params.SubjectID, params.SubjectTypeID)
		if err != nil {
			return errors.Capture(err)
		}

		// Look up the UUID of the access scope entity.
		perm.ScopeTypeID = params.ScopeTypeID
		perm.ScopeUUID, err = st.lookupScopeUUID(ctx, tx, params.ScopeID, params.ScopeTypeID)
		if err != nil {
			return errors.Capture(err)
		}

		// Check that the access scope or subject type is not changing.
		id := secretID{}
		err = tx.Query(ctx, checkInvariantStmt, perm).Get(&id)
		if err == nil {
			// Should never happen.
			return secreterrors.InvalidSecretPermissionChange
		} else if !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("checking duplicate permission record for secret %q: %w", uri, err)
		}

		return st.grantAccess(ctx, tx, perm)
	})
	return errors.Capture(err)
}

// NamespaceForWatchSecretMetadata returns namespace identifier for
// secret metadata watcher.
func (st State) NamespaceForWatchSecretMetadata() string {
	return "secret_metadata"
}

// NamespaceForWatchSecretRevisionObsolete returns namespace identifier for
// obsolete secret revision watcher.
func (st State) NamespaceForWatchSecretRevisionObsolete() string {
	return "secret_revision_obsolete"
}

const (
	selectUnitUUID        = `SELECT uuid AS &entityRef.uuid FROM unit WHERE name=$entityRef.id`
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
		subjectNotFoundError = applicationerrors.UnitNotFound
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
		return "", errors.Capture(err)
	}
	result := entityRef{}
	err = tx.Query(ctx, selectSubjectUUIDStmt, selectSubjectQueryParams...).Get(&result)
	if err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return "", errors.Errorf("%s %q not found", subjectTypeID, subjectID).Add(subjectNotFoundError)
		} else {
			subject := subjectID
			if subjectTypeID == domainsecret.SubjectModel {
				subject = "model"
			}
			return "", errors.Errorf("looking up secret grant subject UUID for %q: %w", subject, err)
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
		scopeNotFoundError = applicationerrors.UnitNotFound
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
		return "", errors.Capture(err)
	}

	result := entityRef{}
	err = tx.Query(ctx, selectScopeUUIDStmt, selectScopeQueryParams...).Get(&result)
	if err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return "", errors.Errorf("%s %q not found", scopeTypeID, scopeID).Add(scopeNotFoundError)
		} else {
			scope := scopeID
			if scopeTypeID == domainsecret.ScopeModel {
				scope = "model"
			}
			return "", errors.Errorf("looking up secret grant scope UUID for %q: %w", scope, err)
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
		return errors.Capture(err)
	}

	if err := tx.Query(ctx, insertStmt, perm).Run(); err != nil {
		return errors.Capture(err)
	}
	return nil

}

// RevokeAccess revokes access to the secret for the specified subject.
// It returns an error satisfying [secreterrors.SecretNotFound] if the
// secret is not found.
func (st State) RevokeAccess(ctx context.Context, uri *coresecrets.URI, params domainsecret.AccessParams) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
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
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if isLocal, err := st.checkExistsIfLocal(ctx, tx, uri); err != nil {
			return errors.Capture(err)
		} else if !isLocal {
			// Should never happen.
			return secreterrors.SecretIsNotLocal
		}

		// Look up the UUID of the subject.
		perm.SubjectUUID, err = st.lookupSubjectUUID(ctx, tx, params.SubjectID, params.SubjectTypeID)
		if err != nil {
			return errors.Capture(err)
		}
		err = tx.Query(ctx, deleteStmt, perm).Run()
		if err != nil {
			return errors.Errorf("deleting secret grant for %q on %q: %w", params.SubjectID, uri, err)
		}
		return nil
	})
	return errors.Capture(err)
}

// GetSecretAccess returns the access to the secret for the specified accessor.
// It returns an error satisfying [secreterrors.SecretNotFound]
// if the secret is not found.
func (st State) GetSecretAccess(
	ctx context.Context, uri *coresecrets.URI, params domainsecret.AccessParams,
) (string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
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
		return "", errors.Capture(err)
	}

	var role string
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if isLocal, err := st.checkExistsIfLocal(ctx, tx, uri); err != nil {
			return errors.Capture(err)
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
		if err != nil {
			return errors.Errorf("looking up secret grant for %q on %q: %w", params.SubjectID, uri, err)
		}
		return nil
	})
	return role, errors.Capture(err)
}

// GetSecretAccessScope returns the access scope for the specified accessor's
// permission on the secret.It returns an error satisfying
// [secreterrors.SecretNotFound] if the secret is not found.
func (st State) GetSecretAccessScope(
	ctx context.Context, uri *coresecrets.URI, params domainsecret.AccessParams,
) (*domainsecret.AccessScope, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
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
		return nil, errors.Capture(err)
	}

	result := secretAccessScope{}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if isLocal, err := st.checkExistsIfLocal(ctx, tx, uri); err != nil {
			return errors.Capture(err)
		} else if !isLocal {
			// Should never happen.
			return secreterrors.SecretIsNotLocal
		}
		err = tx.Query(ctx, selectScopeStmt, access).Get(&result)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"access scope for %q on secret %q not found",
				params.SubjectID, uri).Add(secreterrors.SecretAccessScopeNotFound)

		}
		if err != nil {
			return errors.Errorf("looking up secret access scope for %q on %q: %w", params.SubjectID, uri, err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
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
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	query := `
SELECT (sp.*) AS (&secretAccessor.*),
       (sp.*) AS (&secretAccessScope.*)
FROM   v_secret_permission sp
WHERE  secret_id = $secretID.id
AND    role_id = $secretAccessor.role_id
-- exclude remote applications
AND    subject_type_id != $M.remote_application_type`

	arg := sqlair.M{"remote_application_type": domainsecret.SubjectRemoteApplication}

	selectStmt, err := st.Prepare(query, secretID{}, secretAccessor{}, secretAccessScope{}, arg)
	if err != nil {
		return nil, errors.Capture(err)
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
			return errors.Capture(err)
		} else if !isLocal {
			// Should never happen.
			return secreterrors.SecretIsNotLocal
		}
		err = tx.Query(ctx, selectStmt, secretIDParam, secretRole, arg).GetAll(&accessors, &accessScopes)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		if err != nil {
			return errors.Errorf("looking up secret grants for %q: %w", uri, err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	return accessors.toSecretGrants(accessScopes)
}

// AllSecretGrants returns access details for all local secrets, keyed on secret id.
func (st State) AllSecretGrants(ctx context.Context) (map[string][]domainsecret.GrantParams, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	query := `
SELECT (sp.*) AS (&secretAccessor.*),
       (sp.*) AS (&secretAccessScope.*)
FROM   v_secret_permission sp
`

	selectStmt, err := st.Prepare(query, secretAccessor{}, secretAccessScope{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var (
		accessors    secretAccessors
		accessScopes secretAccessScopes
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, selectStmt).GetAll(&accessors, &accessScopes)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		if err != nil {
			return errors.Errorf("looking up secret grants: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	return accessors.toSecretGrantsBySecret(accessScopes)
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
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
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

	queryStmt, err := st.Prepare(query, append(queryParams, secretInfo{}, secretValueRef{})...)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var revisionResult []*coresecrets.SecretRevisionRef
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		var (
			dbSecrets   secretInfos
			dbValueRefs secretValueRefs
		)
		err = tx.Query(ctx, queryStmt, queryParams...).GetAll(&dbSecrets, &dbValueRefs)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("querying accessible secrets: %w", err)
		}
		revisionResult, err = dbSecrets.toSecretRevisionRef(dbValueRefs)
		return errors.Capture(err)
	}); err != nil {
		return nil, errors.Capture(err)
	}
	return revisionResult, nil
}

// GetSecretRevisionID returns the revision UUID for the specified secret URI and revision,
// or an error satisfying [secreterrors.SecretRevisionNotFound] if the revision is not found.
func (st State) GetSecretRevisionID(ctx context.Context, uri *coresecrets.URI, revision int) (string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	secretRev := secretRevision{
		SecretID: uri.ID,
		Revision: revision,
	}
	stmt, err := st.Prepare(`
SELECT uuid AS &secretRevision.uuid
FROM   secret_revision
WHERE  secret_id = $secretRevision.secret_id
    AND    revision = $secretRevision.revision`, secretRev)
	if err != nil {
		return "", errors.Capture(err)
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, secretRev).Get(&secretRev)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("%w: %s/%d", secreterrors.SecretRevisionNotFound, uri, revision)
		}
		return errors.Capture(err)
	})
	if err != nil {
		return "", errors.Capture(err)
	}
	return secretRev.ID, nil
}

type dbrevisionUUIDs []revisionUUID

// InitialWatchStatementForConsumedSecretsChange returns the initial watch
// statement and the table name for watching consumed secrets.
func (st State) InitialWatchStatementForConsumedSecretsChange(unitName coreunit.Name) (string, eventsource.NamespaceQuery) {
	queryFunc := func(ctx context.Context, runner coredatabase.TxnRunner) ([]string, error) {
		q := `
SELECT   DISTINCT sr.uuid AS &revisionUUID.uuid
FROM     secret_unit_consumer suc
         JOIN unit u ON u.uuid = suc.unit_uuid
         JOIN secret_revision sr ON sr.secret_id = suc.secret_id
WHERE    u.name = $unit.name
GROUP BY sr.secret_id
HAVING   suc.current_revision < MAX(sr.revision)`

		queryParams := []any{
			unit{Name: unitName},
		}

		stmt, err := st.Prepare(q, append(queryParams, revisionUUID{})...)
		if err != nil {
			return nil, errors.Capture(err)
		}

		var revUUIDs dbrevisionUUIDs
		err = runner.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			err := tx.Query(ctx, stmt, queryParams...).GetAll(&revUUIDs)
			if errors.Is(err, sqlair.ErrNoRows) {
				// No consumed secrets found.
				return nil
			}
			return errors.Capture(err)
		})
		if err != nil {
			return nil, errors.Capture(err)
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
	ctx context.Context, unitName coreunit.Name, revisionIDs ...string,
) ([]string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	q := `
SELECT DISTINCT suc.secret_id AS &secretUnitConsumer.secret_id
FROM   secret_unit_consumer suc
       JOIN unit u ON u.uuid = suc.unit_uuid
       JOIN secret_revision sr ON sr.secret_id = suc.secret_id
WHERE  u.name = $unit.name`

	queryParams := []any{
		unit{Name: unitName},
	}

	if len(revisionIDs) > 0 {
		queryParams = append(queryParams, revisionUUIDs(revisionIDs))
		q += " AND sr.uuid IN ($revisionUUIDs[:])"
	}
	q += `
GROUP BY sr.secret_id
HAVING suc.current_revision < MAX(sr.revision)`

	stmt, err := st.Prepare(q, append(queryParams, secretUnitConsumer{})...)
	if err != nil {
		return nil, errors.Capture(err)
	}
	var dbConsumers secretUnitConsumers
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, queryParams...).GetAll(&dbConsumers)
		if errors.Is(err, sqlair.ErrNoRows) {
			// No consumed secrets found.
			return nil
		}
		return errors.Capture(err)
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	secretURIs := make([]string, len(dbConsumers))
	for i, consumer := range dbConsumers {
		uri, err := coresecrets.ParseURI(consumer.SecretID)
		if err != nil {
			return nil, errors.Capture(err)
		}
		secretURIs[i] = uri.String()
	}
	return secretURIs, nil
}

type remoteSecrets []remoteSecret

// InitialWatchStatementForConsumedRemoteSecretsChange returns the initial
// watch statement and the table name for watching consumed secrets hosted
// in a different model.
func (st State) InitialWatchStatementForConsumedRemoteSecretsChange(unitName coreunit.Name) (string, eventsource.NamespaceQuery) {
	queryFunc := func(ctx context.Context, runner coredatabase.TxnRunner) ([]string, error) {
		q := `
SELECT   DISTINCT sr.secret_id AS &remoteSecret.secret_id
FROM     secret_unit_consumer suc
         JOIN unit u ON u.uuid = suc.unit_uuid
         JOIN secret_reference sr ON sr.secret_id = suc.secret_id
WHERE    u.name = $unit.name
GROUP BY sr.secret_id
HAVING   suc.current_revision < sr.latest_revision`

		queryParams := []any{
			unit{Name: unitName},
		}

		stmt, err := st.Prepare(q, append(queryParams, remoteSecret{})...)
		if err != nil {
			return nil, errors.Capture(err)
		}
		var referenceIDs remoteSecrets
		err = runner.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			err := tx.Query(ctx, stmt, queryParams...).GetAll(&referenceIDs)
			if errors.Is(err, sqlair.ErrNoRows) {
				// No consumed remote secrets found.
				return nil
			}
			return errors.Capture(err)
		})
		if err != nil {
			return nil, errors.Capture(err)
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
	ctx context.Context, unitName coreunit.Name, secretIDs ...string,
) ([]string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	q := `
SELECT suc.secret_id AS &secretUnitConsumer.secret_id,
       suc.source_model_uuid AS &secretUnitConsumer.source_model_uuid
FROM   secret_unit_consumer suc
       JOIN unit u ON u.uuid = suc.unit_uuid
       JOIN secret_reference sr ON sr.secret_id = suc.secret_id
WHERE  u.name = $unit.name`

	queryParams := []any{
		unit{Name: unitName},
	}

	if len(secretIDs) > 0 {
		queryParams = append(queryParams, dbSecretIDs(secretIDs))
		q += " AND sr.secret_id IN ($dbSecretIDs[:])"
	}
	q += `
GROUP BY sr.secret_id
HAVING suc.current_revision < sr.latest_revision`

	stmt, err := st.Prepare(q, append(queryParams, secretUnitConsumer{})...)
	if err != nil {
		return nil, errors.Capture(err)
	}
	var consumers secretUnitConsumers
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, queryParams...).GetAll(&consumers)
		if errors.Is(err, sqlair.ErrNoRows) {
			// No consumed secrets found.
			return nil
		}
		return errors.Capture(err)
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	secretURIs := make([]string, len(consumers))
	for i, consumer := range consumers {
		uri, err := coresecrets.ParseURI(consumer.SecretID)
		if err != nil {
			return nil, errors.Capture(err)
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
		// WHERE sruc.unit_name LIKE '$M.app_name/%'`

		q := fmt.Sprintf(`
SELECT DISTINCT sr.uuid AS &revisionUUID.uuid
FROM   secret_remote_unit_consumer sruc
       LEFT JOIN secret_revision sr ON sr.secret_id = sruc.secret_id
WHERE  sruc.unit_name LIKE '%s/%%'`, appName)

		queryParams := []any{
			// TODO: enable this once https://github.com/canonical/sqlair/issues/148 is fixed.
			// sqlair.M{"app_name": appName},
		}
		q += `
GROUP BY sruc.secret_id
HAVING sruc.current_revision < MAX(sr.revision)`
		stmt, err := st.Prepare(q, append(queryParams, revisionUUID{})...)
		if err != nil {
			return nil, errors.Capture(err)
		}
		var revisionUUIDs dbrevisionUUIDs
		err = runner.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			err := tx.Query(ctx, stmt, queryParams...).GetAll(&revisionUUIDs)
			if errors.Is(err, sqlair.ErrNoRows) {
				// No consumed secrets found.
				return nil
			}
			return errors.Capture(err)
		})
		if err != nil {
			return nil, errors.Capture(err)
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
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	// TODO: sqlair does not support inject parameters into values in quotation marks.
	// Use sqlair to generate the query once https://github.com/canonical/sqlair/issues/148 is fixed.
	// q := `
	// SELECT DISTINCT sruc.secret_id AS &secretRemoteUnitConsumer.secret_id
	// FROM secret_remote_unit_consumer sruc
	// LEFT JOIN secret_revision sr ON sr.secret_id = sruc.secret_id
	// WHERE sruc.unit_name LIKE '$M.app_name/%'`

	q := fmt.Sprintf(`
SELECT DISTINCT sruc.secret_id AS &secretRemoteUnitConsumer.secret_id
FROM   secret_remote_unit_consumer sruc
       LEFT JOIN secret_revision sr ON sr.secret_id = sruc.secret_id
WHERE  sruc.unit_name LIKE '%s/%%'`, appName)

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
	stmt, err := st.Prepare(q, append(queryParams, secretRemoteUnitConsumer{})...)
	if err != nil {
		return nil, errors.Capture(err)
	}
	var remoteConsumers secretRemoteUnitConsumers
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, queryParams...).GetAll(&remoteConsumers)
		if errors.Is(err, sqlair.ErrNoRows) {
			// No consumed secrets found.
			return nil
		}
		return errors.Capture(err)
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	modelUUID, err := st.GetModelUUID(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	secretURIs := make([]string, len(remoteConsumers))
	for i, consumer := range remoteConsumers {
		uri, err := coresecrets.ParseURI(consumer.SecretID)
		if err != nil {
			return nil, errors.Capture(err)
		}
		// We need to set the source model UUID to mark it as a remote secret for consumer side to use.
		uri.SourceUUID = modelUUID.String()
		secretURIs[i] = uri.String()
	}
	return secretURIs, nil
}

type dbSecretIDs []string

// InitialWatchStatementForObsoleteRevision returns the initial watch statement
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
			return nil, errors.Capture(err)
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
// or unitOwners must be specified. It returns a map of revision UUIDs
// to their corresponding secret IDs.
func (st State) GetRevisionIDsForObsolete(
	ctx context.Context,
	appOwners domainsecret.ApplicationOwners,
	unitOwners domainsecret.UnitOwners,
	revisionUUIDs ...string,
) (map[string]string, error) {
	if len(revisionUUIDs) == 0 && len(appOwners) == 0 && len(unitOwners) == 0 {
		return nil, nil
	}
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var revisions []secretRevision
	if err := st.getRevisionForObsolete(
		ctx, db, `
sr.secret_id AS &secretRevision.secret_id,
sr.revision AS &secretRevision.revision,
sro.revision_uuid AS &secretRevision.uuid`, secretRevision{}, &revisions,
		appOwners, unitOwners, revisionUUIDs...,
	); err != nil {
		return nil, errors.Capture(err)
	}
	result := make(map[string]string, len(revisions))
	for _, rev := range revisions {
		result[rev.ID] = getRevisionID(rev.SecretID, rev.Revision)
	}
	return result, nil
}

func (st State) getRevisionForObsolete(
	ctx context.Context, runner domain.TxnRunner,
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
		joins = append(joins, `
     LEFT JOIN secret_application_owner sao ON sr.secret_id = sao.secret_id
     LEFT JOIN application ON application.uuid = sao.application_uuid
     LEFT JOIN secret_unit_owner suo ON sr.secret_id = suo.secret_id
     LEFT JOIN unit ON unit.uuid = suo.unit_uuid`[1:],
		)
		conditions = append(conditions, `AND (
    sao.application_uuid IS NOT NULL AND application.name IN ($ApplicationOwners[:])
    OR suo.unit_uuid IS NOT NULL AND unit.name IN ($UnitOwners[:])
)`)
	} else if len(appOwners) > 0 {
		queryParams = append(queryParams, appOwners)
		joins = append(joins, `
     LEFT JOIN secret_application_owner sao ON sr.secret_id = sao.secret_id
     LEFT JOIN application ON application.uuid = sao.application_uuid`[1:],
		)
		conditions = append(conditions, "AND sao.application_uuid IS NOT NULL AND application.name IN ($ApplicationOwners[:])")
	} else if len(unitOwners) > 0 {
		queryParams = append(queryParams, unitOwners)
		joins = append(joins, `
     LEFT JOIN secret_unit_owner suo ON sr.secret_id = suo.secret_id
     LEFT JOIN unit ON unit.uuid = suo.unit_uuid`[1:],
		)
		conditions = append(conditions, "AND suo.unit_uuid IS NOT NULL AND unit.name IN ($UnitOwners[:])")
	}
	if len(joins) > 0 {
		q += fmt.Sprintf("\n%s", strings.Join(joins, "\n"))
	}
	if len(conditions) > 0 {
		q += fmt.Sprintf("\nWHERE %s", strings.Join(conditions, "\n"))
	}
	st.logger.Tracef(ctx,
		"revisionUUIDs %+v, appOwners: %+v, unitOwners: %+v, query: \n%s",
		revUUIDs, appOwners, unitOwners, q,
	)
	stmt, err := st.Prepare(q, append(queryParams, outputType)...)
	if err != nil {
		return errors.Capture(err)
	}
	err = runner.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, queryParams...).GetAll(result)
		if errors.Is(err, sqlair.ErrNoRows) {
			// It's ok, the revisions probably have already been pruned.
			return nil
		}
		return errors.Capture(err)
	})
	return errors.Capture(err)
}

type (
	revisions     []int
	revisionUUIDs []string
)

// DeleteSecret deletes the specified secret revisions.
// If revisions is nil the last remaining revisions are removed.
func (st State) DeleteSecret(ctx domain.AtomicContext, uri *coresecrets.URI, revs []int) error {
	err := domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		_, err = st.deleteSecretRevisions(ctx, tx, uri, revs)
		return errors.Capture(err)
	})
	if err != nil {
		return errors.Capture(err)
	}
	return nil
}

// DeleteObsoleteUserSecretRevisions deletes the obsolete user secret revisions.
// It returns the string format UUID of the deleted revisions.
func (st State) DeleteObsoleteUserSecretRevisions(ctx context.Context) ([]string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	q := `
SELECT smo.secret_id AS &secretID.id,
       sr.revision AS &secretExternalRevision.revision
FROM   secret_model_owner smo
       JOIN secret_metadata sm ON sm.secret_id = smo.secret_id
       JOIN secret_revision sr ON sr.secret_id = smo.secret_id
       LEFT JOIN secret_revision_obsolete sro ON sro.revision_uuid = sr.uuid
WHERE  sm.auto_prune = true AND sro.obsolete = true`

	stmt, err := st.Prepare(q, secretID{}, secretExternalRevision{})
	if err != nil {
		return nil, errors.Capture(err)
	}
	var deletedRevisionIDs []string
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var (
			dbSecrets    secretIDs
			dbsecretRevs secretExternalRevisions
		)
		err = tx.Query(ctx, stmt).GetAll(&dbSecrets, &dbsecretRevs)
		if errors.Is(err, sqlair.ErrNoRows) {
			// Nothing to delete.
			return nil
		}
		if err != nil {
			return errors.Capture(err)
		}
		itemsToDelete, err := dbSecrets.toSecretMetadataForDrain(dbsecretRevs)
		if err != nil {
			return errors.Capture(err)
		}
		for _, toDelete := range itemsToDelete {
			revs := make([]int, len(toDelete.Revisions))
			for i, r := range toDelete.Revisions {
				revs[i] = r.Revision
			}
			deleted, err := st.deleteSecretRevisions(ctx, tx, toDelete.URI, revs)
			if err != nil {
				return errors.Capture(err)
			}
			deletedRevisionIDs = append(deletedRevisionIDs, deleted...)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	return deletedRevisionIDs, nil
}

// deleteSecretRevisions deletes the specified secret revisions, or all if revs is nil.
// If the last remaining revisions are removed, the secret is deleted.
func (st State) deleteSecretRevisions(ctx context.Context, tx *sqlair.TX, uri *coresecrets.URI, revs []int) ([]string, error) {
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
	selectRevisionStmt, err := st.Prepare(selectRevsToDelete, append(selectRevisionParams, revisionUUID{})...)
	if err != nil {
		return nil, errors.Capture(err)
	}

	countRevisions := `SELECT count(*) AS &M.count FROM secret_revision WHERE secret_id = $secretID.id`
	countRevisionsStmt, err := st.Prepare(countRevisions, secretID{}, sqlair.M{})
	if err != nil {
		return nil, errors.Capture(err)
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
			return nil, errors.Capture(err)
		}
	}

	if isLocal, err := st.checkExistsIfLocal(ctx, tx, uri); err != nil {
		return nil, errors.Capture(err)
	} else if !isLocal {
		// Should never happen.
		return nil, secreterrors.SecretIsNotLocal
	}

	result := []revisionUUID{}
	err = tx.Query(ctx, selectRevisionStmt, selectRevisionParams...).GetAll(&result)
	if errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Errorf("secret revisions %v not found", revs).Add(secreterrors.SecretRevisionNotFound)
	}
	if err != nil {
		return nil, errors.Errorf("selecting revision UUIDs to delete for secret %q: %w", uri, err)
	}

	toDelete := make(revisionUUIDs, len(result))
	for i, r := range result {
		toDelete[i] = r.UUID
	}
	for _, stmt := range deleteRevisionStmts {
		err = tx.Query(ctx, stmt, toDelete).Run()
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return nil, errors.Errorf("deleting revision info for secret %q: %w", uri, err)
		}
	}

	countResult := sqlair.M{}
	err = tx.Query(ctx, countRevisionsStmt, selectRevisionParams[0]).Get(&countResult)
	if err != nil {
		return nil, errors.Errorf("counting remaining revisions for secret %q: %w", uri, err)
	}
	count, _ := strconv.Atoi(fmt.Sprint(countResult["count"]))
	if count > 0 {
		return toDelete, nil
	}
	// No revisions left so delete the secret.
	if err := st.deleteSecret(ctx, tx, uri); err != nil {
		return nil, errors.Capture(err)
	}
	return toDelete, nil
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
			return errors.Capture(err)
		}
	}

	for _, stmt := range deleteSecretStmts {
		err = tx.Query(ctx, stmt, secretIDParamParam).Run()
		if err != nil {
			return errors.Errorf("deleting info for secret %q: %w", uri, err)
		}
	}
	return nil
}

// SecretRotated updates the next rotation time for the specified secret.
func (st State) SecretRotated(ctx context.Context, uri *coresecrets.URI, next time.Time) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := st.upsertSecretNextRotateTime(ctx, tx, uri, next)
		return errors.Capture(err)
	})
	return errors.Capture(err)
}

func (st State) getSecretsRotationChanges(
	ctx context.Context, runner domain.TxnRunner,
	appOwners domainsecret.ApplicationOwners, unitOwners domainsecret.UnitOwners,
	secretIDs ...string,
) ([]domainsecret.RotationInfo, error) {
	if len(secretIDs) == 0 && len(appOwners) == 0 && len(unitOwners) == 0 {
		return nil, nil
	}

	q := `
SELECT
       sro.secret_id AS &secretRotationChange.secret_id,
       sro.next_rotation_time AS &secretRotationChange.next_rotation_time,
       MAX(sr.revision) AS &secretRotationChange.revision
FROM   secret_rotation sro
       JOIN secret_revision sr ON sr.secret_id = sro.secret_id`

	var queryParams []any
	var joins []string
	conditions := []string{}
	if len(secretIDs) > 0 {
		queryParams = append(queryParams, dbSecretIDs(secretIDs))
		conditions = append(conditions, "sro.secret_id IN ($dbSecretIDs[:])")
	}
	if len(appOwners) > 0 && len(unitOwners) > 0 {
		queryParams = append(queryParams, appOwners, unitOwners)
		joins = append(joins, `
        LEFT JOIN secret_application_owner sao ON sro.secret_id = sao.secret_id
        LEFT JOIN application ON application.uuid = sao.application_uuid
        LEFT JOIN secret_unit_owner suo ON sro.secret_id = suo.secret_id
        LEFT JOIN unit ON unit.uuid = suo.unit_uuid`[1:],
		)
		conditions = append(conditions, `(
    sao.application_uuid IS NOT NULL AND application.name IN ($ApplicationOwners[:])
    OR suo.unit_uuid IS NOT NULL AND unit.name IN ($UnitOwners[:])
)`)
	} else if len(appOwners) > 0 {
		queryParams = append(queryParams, appOwners)
		joins = append(joins, `
        LEFT JOIN secret_application_owner sao ON sro.secret_id = sao.secret_id
        LEFT JOIN application ON application.uuid = sao.application_uuid`[1:],
		)
		conditions = append(conditions, "sao.application_uuid IS NOT NULL AND application.name IN ($ApplicationOwners[:])")
	} else if len(unitOwners) > 0 {
		queryParams = append(queryParams, unitOwners)
		joins = append(joins, `
        LEFT JOIN secret_unit_owner suo ON sro.secret_id = suo.secret_id
        LEFT JOIN unit ON unit.uuid = suo.unit_uuid`[1:],
		)
		conditions = append(conditions, "suo.unit_uuid IS NOT NULL AND unit.name IN ($UnitOwners[:])")
	}
	if len(joins) > 0 {
		q += fmt.Sprintf("\n%s", strings.Join(joins, "\n"))
	}
	if len(conditions) > 0 {
		q += fmt.Sprintf("\nWHERE %s", strings.Join(conditions, "\nAND "))
	}
	q += `
GROUP BY sro.secret_id`
	st.logger.Tracef(ctx,
		"secretIDs %+v, appOwners: %+v, unitOwners: %+v, query: \n%s",
		secretIDs, appOwners, unitOwners, q,
	)

	stmt, err := st.Prepare(q, append(queryParams, secretRotationChange{})...)
	if err != nil {
		return nil, errors.Capture(err)
	}
	var data []secretRotationChange
	err = runner.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, queryParams...).GetAll(&data)
		if errors.Is(err, sqlair.ErrNoRows) {
			// It's ok because the secret or the rotation was just deleted.
			return nil
		}
		return errors.Capture(err)
	})

	if err != nil {
		return nil, errors.Capture(err)
	}
	result := make([]domainsecret.RotationInfo, len(data))
	for i, d := range data {
		result[i] = domainsecret.RotationInfo{
			Revision:        d.Revision,
			NextTriggerTime: d.NextRotateTime,
		}
		uri, err := coresecrets.ParseURI(d.SecretID)
		if err != nil {
			return nil, errors.Capture(err)
		}
		result[i].URI = uri
	}
	return result, nil
}

// ChangeSecretBackend changes the secret backend for the specified secret.
func (st State) ChangeSecretBackend(
	ctx context.Context, revisionID uuid.UUID,
	valueRef *coresecrets.ValueRef, data coresecrets.SecretData,
) (err error) {
	if valueRef != nil && len(data) > 0 {
		return errors.New("both valueRef and data cannot be set")
	}
	if valueRef == nil && len(data) == 0 {
		return errors.New("either valueRef or data must be set")
	}
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	input := revisionUUID{
		UUID: revisionID.String(),
	}

	deleteValueRefQ, err := st.Prepare(`
DELETE FROM secret_value_ref
WHERE revision_uuid = $revisionUUID.uuid`, input)
	if err != nil {
		return errors.Capture(err)
	}
	deleteDataQ, err := st.Prepare(`
DELETE FROM secret_content
WHERE revision_uuid = $revisionUUID.uuid`, input)
	if err != nil {
		return errors.Capture(err)
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if valueRef != nil {
			if err := st.upsertSecretValueRef(ctx, tx, input.UUID, valueRef); err != nil {
				return errors.Capture(err)
			}
		} else {
			if err = tx.Query(ctx, deleteValueRefQ, input).Run(); err != nil {
				return errors.Capture(err)
			}
		}
		if len(data) > 0 {
			if err := st.updateSecretContent(ctx, tx, input.UUID, data); err != nil {
				return errors.Capture(err)
			}
		} else {
			if err = tx.Query(ctx, deleteDataQ, input).Run(); err != nil {
				return errors.Capture(err)
			}
		}
		return errors.Capture(err)
	})
	if err != nil {
		return errors.Capture(err)
	}
	return nil
}

// InitialWatchStatementForSecretsRotationChanges returns the initial watch statement
// and the table name for watching rotations.
func (st State) InitialWatchStatementForSecretsRotationChanges(
	appOwners domainsecret.ApplicationOwners, unitOwners domainsecret.UnitOwners,
) (string, eventsource.NamespaceQuery) {
	queryFunc := func(ctx context.Context, runner coredatabase.TxnRunner) ([]string, error) {
		result, err := st.getSecretsRotationChanges(ctx, runner, appOwners, unitOwners)
		if err != nil {
			return nil, errors.Capture(err)
		}
		secretIDs := make([]string, len(result))
		for i, d := range result {
			secretIDs[i] = d.URI.ID
		}
		return secretIDs, nil
	}
	return "secret_rotation", queryFunc
}

// GetSecretsRotationChanges returns the rotation changes for the owners' secrets.
func (st State) GetSecretsRotationChanges(
	ctx context.Context, appOwners domainsecret.ApplicationOwners, unitOwners domainsecret.UnitOwners, secretIDs ...string,
) ([]domainsecret.RotationInfo, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return st.getSecretsRotationChanges(ctx, db, appOwners, unitOwners, secretIDs...)
}

func (st State) getSecretsRevisionExpiryChanges(
	ctx context.Context, runner domain.TxnRunner,
	appOwners domainsecret.ApplicationOwners, unitOwners domainsecret.UnitOwners,
	revisionIDs ...string,
) ([]domainsecret.ExpiryInfo, error) {
	if len(revisionIDs) == 0 && len(appOwners) == 0 && len(unitOwners) == 0 {
		return nil, nil
	}

	q := `
SELECT
       sr.secret_id AS &secretRevisionExpireChange.secret_id,
       sre.revision_uuid AS &secretRevisionExpireChange.revision_uuid,
       sre.expire_time AS &secretRevisionExpireChange.expire_time,
       MAX(sr.revision) AS &secretRevisionExpireChange.revision
FROM   secret_revision_expire sre
       JOIN secret_revision sr ON sr.uuid = sre.revision_uuid`

	var queryParams []any
	var joins []string
	conditions := []string{}
	if len(revisionIDs) > 0 {
		queryParams = append(queryParams, revisionUUIDs(revisionIDs))
		conditions = append(conditions, "sre.revision_uuid IN ($revisionUUIDs[:])")
	}
	if len(appOwners) > 0 && len(unitOwners) > 0 {
		queryParams = append(queryParams, appOwners, unitOwners)
		joins = append(joins, `
        LEFT JOIN secret_application_owner sao ON sr.secret_id = sao.secret_id
        LEFT JOIN application ON application.uuid = sao.application_uuid
        LEFT JOIN secret_unit_owner suo ON sr.secret_id = suo.secret_id
        LEFT JOIN unit ON unit.uuid = suo.unit_uuid`[1:],
		)
		conditions = append(conditions, `(
    sao.application_uuid IS NOT NULL AND application.name IN ($ApplicationOwners[:])
    OR suo.unit_uuid IS NOT NULL AND unit.name IN ($UnitOwners[:])
)`)
	} else if len(appOwners) > 0 {
		queryParams = append(queryParams, appOwners)
		joins = append(joins, `
        LEFT JOIN secret_application_owner sao ON sr.secret_id = sao.secret_id
        LEFT JOIN application ON application.uuid = sao.application_uuid`[1:],
		)
		conditions = append(conditions, "sao.application_uuid IS NOT NULL AND application.name IN ($ApplicationOwners[:])")
	} else if len(unitOwners) > 0 {
		queryParams = append(queryParams, unitOwners)
		joins = append(joins, `
        LEFT JOIN secret_unit_owner suo ON sr.secret_id = suo.secret_id
        LEFT JOIN unit ON unit.uuid = suo.unit_uuid`[1:],
		)
		conditions = append(conditions, "suo.unit_uuid IS NOT NULL AND unit.name IN ($UnitOwners[:])")
	}
	if len(joins) > 0 {
		q += fmt.Sprintf("\n%s", strings.Join(joins, "\n"))
	}
	if len(conditions) > 0 {
		q += fmt.Sprintf("\nWHERE %s", strings.Join(conditions, "\nAND "))
	}
	q += `
GROUP BY sr.secret_id`
	st.logger.Tracef(ctx,
		"revisionIDs %+v, appOwners: %+v, unitOwners: %+v, query: \n%s",
		revisionIDs, appOwners, unitOwners, q,
	)

	stmt, err := st.Prepare(q, append(queryParams, secretRevisionExpireChange{})...)
	if err != nil {
		return nil, errors.Capture(err)
	}
	var data []secretRevisionExpireChange
	err = runner.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, queryParams...).GetAll(&data)
		if errors.Is(err, sqlair.ErrNoRows) {
			// It's ok because the secret or the expiry was just deleted.
			return nil
		}
		return errors.Capture(err)
	})

	if err != nil {
		return nil, errors.Capture(err)
	}
	result := make([]domainsecret.ExpiryInfo, len(data))
	for i, d := range data {
		result[i] = domainsecret.ExpiryInfo{
			RevisionID:      d.RevisionUUID,
			Revision:        d.Revision,
			NextTriggerTime: d.ExpireTime,
		}
		uri, err := coresecrets.ParseURI(d.SecretID)
		if err != nil {
			return nil, errors.Capture(err)
		}
		result[i].URI = uri
	}
	return result, nil
}

// InitialWatchStatementForSecretsRevisionExpiryChanges returns the initial watch statement
// and the table name for watching secret revision expiry changes.
func (st State) InitialWatchStatementForSecretsRevisionExpiryChanges(
	appOwners domainsecret.ApplicationOwners, unitOwners domainsecret.UnitOwners,
) (string, eventsource.NamespaceQuery) {
	queryFunc := func(ctx context.Context, runner coredatabase.TxnRunner) ([]string, error) {
		result, err := st.getSecretsRevisionExpiryChanges(ctx, runner, appOwners, unitOwners)
		if err != nil {
			return nil, errors.Capture(err)
		}
		revisionUUIDs := make([]string, len(result))
		for i, d := range result {
			revisionUUIDs[i] = d.RevisionID
		}
		return revisionUUIDs, nil
	}
	return "secret_revision_expire", queryFunc
}

// GetSecretsRevisionExpiryChanges returns the expiry changes for the owners' secret revisions.
func (st State) GetSecretsRevisionExpiryChanges(
	ctx context.Context, appOwners domainsecret.ApplicationOwners, unitOwners domainsecret.UnitOwners, revisionUUIDs ...string,
) ([]domainsecret.ExpiryInfo, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return st.getSecretsRevisionExpiryChanges(ctx, db, appOwners, unitOwners, revisionUUIDs...)
}

// GetObsoleteUserSecretRevisionReadyToPrune returns the specified user secret revision with secret ID if it is ready to prune.
func (st State) GetObsoleteUserSecretRevisionsReadyToPrune(ctx context.Context) ([]string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	q := `
SELECT (sr.revision, sr.secret_id) AS (&obsoleteRevisionRow.*)
FROM   secret_model_owner smo
       JOIN secret_metadata sm ON sm.secret_id = smo.secret_id
       JOIN secret_revision sr ON sr.secret_id = smo.secret_id
       LEFT JOIN secret_revision_obsolete sro ON sro.revision_uuid = sr.uuid
WHERE  sm.auto_prune = true AND sro.obsolete = true`
	stmt, err := st.Prepare(q, obsoleteRevisionRow{})
	if err != nil {
		return nil, errors.Capture(err)
	}
	var result obsoleteRevisionRows
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).GetAll(&result)
		if errors.Is(err, sqlair.ErrNoRows) {
			// It's ok, the revision probably has already been pruned.
			return nil
		}
		return errors.Capture(err)
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	return result.toRevIDs(), nil
}
