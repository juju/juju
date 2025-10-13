// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	coredatabase "github.com/juju/juju/core/database"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/watcher/eventsource"
	modelerrors "github.com/juju/juju/domain/model/errors"
	secreterrors "github.com/juju/juju/domain/secret/errors"
	"github.com/juju/juju/internal/errors"
)

type (
	revisionUUIDs   []string
	dbrevisionUUIDs []revisionUUID
)

// InitialWatchStatementForRemoteConsumedSecretsChangesFromOfferingSide
// returns the initial watch statement and the table name for watching
// remote consumed secrets.
func (st *State) InitialWatchStatementForRemoteConsumedSecretsChangesFromOfferingSide(
	appUUID string,
) (string, eventsource.NamespaceQuery) {
	queryFunc := func(ctx context.Context, runner coredatabase.TxnRunner) ([]string, error) {
		q := `
SELECT DISTINCT sr.uuid AS &revisionUUID.uuid
FROM      secret_remote_unit_consumer sruc
LEFT JOIN secret_revision sr ON sr.secret_id = sruc.secret_id
JOIN      application app ON app.name = substr(sruc.unit_name, 1, instr(sruc.unit_name, '/')-1)
WHERE     app.uuid = $applicationUUID.uuid
GROUP BY  sruc.secret_id
HAVING    sruc.current_revision < MAX(sr.revision)`
		app := applicationUUID{UUID: appUUID}
		stmt, err := st.Prepare(q, app, revisionUUID{})
		if err != nil {
			return nil, errors.Capture(err)
		}
		var revisionUUIDs dbrevisionUUIDs
		err = runner.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			err := tx.Query(ctx, stmt, app).GetAll(&revisionUUIDs)
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

func (st *State) getModelUUID(ctx context.Context, tx *sqlair.TX) (string, error) {
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

// GetRemoteConsumedSecretURIsWithChangesFromOfferingSide returns the URIs
// of the secrets consumed by the specified remote application that has new
// revisions.
func (st *State) GetRemoteConsumedSecretURIsWithChangesFromOfferingSide(
	ctx context.Context, appUUID string, revUUIDs ...string,
) ([]string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	q := `
SELECT DISTINCT sruc.secret_id AS &secretRemoteUnitConsumer.secret_id
FROM      secret_remote_unit_consumer sruc
LEFT JOIN secret_revision sr ON sr.secret_id = sruc.secret_id
JOIN      application app ON app.name = substr(sruc.unit_name, 1, instr(sruc.unit_name, '/')-1)
WHERE     app.uuid = $applicationUUID.uuid`
	queryParams := []any{
		applicationUUID{UUID: appUUID},
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
	var (
		remoteConsumers secretRemoteUnitConsumers
		modelUUID       string
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, queryParams...).GetAll(&remoteConsumers)
		if errors.Is(err, sqlair.ErrNoRows) {
			// No consumed secrets found.
			return nil
		}
		modelUUID, err = st.getModelUUID(ctx, tx)
		if err != nil {
			return errors.Capture(err)
		}
		return errors.Capture(err)
	})
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
		uri.SourceUUID = modelUUID
		secretURIs[i] = uri.String()
	}
	return secretURIs, nil
}

// checkExists an error satisfying [secreterrors.SecretNotFound] if the
// specified secret URI does not exist in the model.
func (st *State) checkExists(ctx context.Context, tx *sqlair.TX, uri *coresecrets.URI) error {
	ref := secretRef{ID: uri.ID, SourceUUID: uri.SourceUUID}
	queryStmt, err := st.Prepare(`
SELECT secret_id AS &secretRef.secret_id FROM secret_metadata sm
WHERE  sm.secret_id = $secretRef.secret_id
AND    ($secretRef.source_uuid = '' OR $secretRef.source_uuid = (SELECT uuid FROM model))
`, ref)
	if err != nil {
		return errors.Capture(err)
	}
	var result secretRef
	err = tx.Query(ctx, queryStmt, ref).Get(&result)
	if errors.Is(err, sqlair.ErrNoRows) {
		return secreterrors.SecretNotFound
	}
	if err != nil {
		return errors.Errorf("looking up secret URI %q: %w", uri, err)
	}
	return nil
}

// GetSecretRemoteConsumer returns the secret consumer info from a cross model consumer
// for the specified unit and secret.
// If the secret does not exist, an error satisfying [secreterrors.SecretNotFound] is returned.
// If there's not currently a consumer record for the secret, the latest revision is still returned,
// along with an error satisfying [secreterrors.SecretConsumerNotFound].
func (st *State) GetSecretRemoteConsumer(
	ctx context.Context, uri *coresecrets.URI, unitName string,
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
SELECT MAX(revision) AS &secretLatestRevision.latest_revision
FROM   secret_revision rev
WHERE  rev.secret_id = $secretLatestRevision.secret_id`
	selectLatestRevisionStmt, err := st.Prepare(selectLatestRevision, secretLatestRevision{})
	if err != nil {
		return nil, 0, errors.Capture(err)
	}

	var (
		dbSecretConsumers secretRemoteUnitConsumers
		latestRevision    int
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := st.checkExists(ctx, tx, uri); err != nil {
			return errors.Capture(err)
		}

		err = tx.Query(ctx, queryStmt, consumer).GetAll(&dbSecretConsumers)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("querying secret consumer info for secret %q and unit %q: %w", uri, unitName, err)
		}

		result := secretLatestRevision{ID: uri.ID}
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
func (st *State) SaveSecretRemoteConsumer(ctx context.Context, uri *coresecrets.URI, unitName string, md coresecrets.SecretConsumerMetadata) error {
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
		if err := st.checkExists(ctx, tx, uri); err != nil {
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

// markObsoleteRevisions obsoletes the revisions and sets the pending_delete
// to true in the secret_revision table for the specified secret if the
// revision is not the latest revision and there are no consumers for the
// revision.
func (st *State) markObsoleteRevisions(ctx context.Context, tx *sqlair.TX, uri *coresecrets.URI) error {
	query, err := st.Prepare(`
SELECT sr.uuid AS &revisionUUID.uuid
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
`, secretRef{}, revisionUUID{})
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
			ID:            revisionUUID.UUID,
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

// UpdateRemoteSecretRevision records the latest revision
// of the specified cross model secret.
func (st *State) UpdateRemoteSecretRevision(ctx context.Context, uri *coresecrets.URI, latestRevision int) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	insertQuery := `
INSERT INTO secret (id)
VALUES ($secretRef.secret_id)
ON CONFLICT(id) DO NOTHING`

	insertStmt, err := st.Prepare(insertQuery, secretRef{})
	if err != nil {
		return errors.Capture(err)
	}

	insertLatestQuery := `
INSERT INTO secret_reference (*)
VALUES ($secretLatestRevision.*)
ON CONFLICT(secret_id) DO UPDATE SET
    latest_revision=excluded.latest_revision`

	insertLatestStmt, err := st.Prepare(insertLatestQuery, secretLatestRevision{})
	if err != nil {
		return errors.Capture(err)
	}

	secret := secretLatestRevision{
		ID:             uri.ID,
		LatestRevision: latestRevision,
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, insertStmt, secretRef{ID: uri.ID}).Run()
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
