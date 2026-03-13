// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"
	"fmt"
	"strconv"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"

	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/internal/errors"
)

// TODO (manadart 2025-10-24): The first two methods should not exist here.
// They should be a function of the Juju secret back-end implementation.
//
// Unfortunately the poor design decision was made to make the Juju (differently
// to K8s and Vault) secret back-end effectively unimplemented, with error
// returns to be interpreted by the caller as a direction to logically branch
// inside state methods, and manipulate secret content there.
//
// Why is this bad? Because everywhere that the back-end is recruited, there
// needs to be an interpretation of the errors and a fall-back implementation
// of whatever we wanted the back-end to do for us - snowflakes everywhere.
//
// What should be done is all methods on the Juju secret back-end and back-end
// provider should be populated with a Dqlite-backed implementation. Following
// that, all methods that manipulate Dqlite for secret content can be removed
// from domains other than from the one that would back the Juju secret
// back-end.

// DeleteApplicationOwnedSecretContent deletes content for all
// secrets owned by the application with the input UUID.
// It must only be called in the context of application removal.
func (st *State) DeleteApplicationOwnedSecretContent(ctx context.Context, aUUID string) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	appUUID := entityUUID{UUID: aUUID}

	q := `
WITH revisions AS (
    SELECT r.uuid
    FROM   secret_application_owner o JOIN secret_revision r ON o.secret_id = o.secret_id
    WHERE  application_uuid = $entityUUID.uuid
)
DELETE FROM secret_content WHERE revision_uuid IN (SELECT uuid FROM revisions)`

	stmt, err := st.Prepare(q, appUUID)
	if err != nil {
		return errors.Errorf("preparing app secret content deletion: %w", err)
	}

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, appUUID).Run(); err != nil {
			return errors.Errorf("running app secret content deletion: %w", err)
		}
		return nil
	}))
}

// DeleteUnitOwnedSecretContent deletes content for all
// secrets owned by the unit with the input UUID.
// It must only be called in the context of unit removal.
func (st *State) DeleteUnitOwnedSecretContent(ctx context.Context, uUUID string) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	unitUUID := entityUUID{UUID: uUUID}

	q := `
WITH revisions AS (
    SELECT r.uuid
    FROM   secret_unit_owner o JOIN secret_revision r ON o.secret_id = o.secret_id
    WHERE  unit_uuid = $entityUUID.uuid
)
DELETE FROM secret_content WHERE revision_uuid IN (SELECT uuid FROM revisions)`

	stmt, err := st.Prepare(q, unitUUID)
	if err != nil {
		return errors.Errorf("preparing unit secret content deletion: %w", err)
	}

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, unitUUID).Run(); err != nil {
			return errors.Errorf("running unit secret content deletion: %w", err)
		}
		return nil
	}))
}

// GetApplicationOwnedSecretRevisionRefs returns the back-end value references
// for secret revisions owned by the application with the input UUID.
func (st *State) GetApplicationOwnedSecretRevisionRefs(ctx context.Context, aUUID string) ([]string, error) {
	appUUID := entityUUID{UUID: aUUID}

	q := `
SELECT vr.revision_id AS &entityUUID.uuid
FROM   secret_application_owner o
       JOIN secret_revision r ON o.secret_id = r.secret_id
	   JOIN secret_value_ref vr ON r.uuid = vr.revision_uuid
WHERE  o.application_uuid = $entityUUID.uuid`
	stmt, err := st.Prepare(q, appUUID)
	if err != nil {
		return nil, errors.Errorf("preparing revision ID query: %w", err)
	}

	refs, err := st.getSecretRevisionRefsForStmt(ctx, stmt, appUUID)
	return refs, errors.Capture(err)
}

// GetUnitOwnedSecretRevisionRefs returns the back-end value references
// for secret revisions owned by the application with the input UUID.
func (st *State) GetUnitOwnedSecretRevisionRefs(ctx context.Context, uUUID string) ([]string, error) {
	appUUID := entityUUID{UUID: uUUID}

	q := `
SELECT vr.revision_id AS &entityUUID.uuid
FROM   secret_unit_owner o
       JOIN secret_revision r ON o.secret_id = r.secret_id
	   JOIN secret_value_ref vr ON r.uuid = vr.revision_uuid
WHERE  o.unit_uuid = $entityUUID.uuid`
	stmt, err := st.Prepare(q, appUUID)
	if err != nil {
		return nil, errors.Errorf("preparing revision ID query: %w", err)
	}

	refs, err := st.getSecretRevisionRefsForStmt(ctx, stmt, appUUID)
	return refs, errors.Capture(err)
}

func (st *State) getSecretRevisionRefsForStmt(
	ctx context.Context, stmt *sqlair.Statement, id entityUUID,
) ([]string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var uuids []entityUUID
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, id).GetAll(&uuids); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("running revision ID query: %w", err)
		}
		return nil
	}); err != nil {
		return nil, errors.Capture(err)
	}

	return transform.Slice(uuids, func(e entityUUID) string { return e.UUID }), nil
}

// DeleteApplicationOwnedSecrets deletes all data for secrets owned by the
// application with the input UUID.
// This does not include secret content, which should be handled by
// interaction with the secret back-end.
func (st *State) DeleteApplicationOwnedSecrets(ctx context.Context, aUUID string) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	appUUID := entityUUID{UUID: aUUID}

	// We need to materialise this up-front, because once we delete the
	// application_secret_owner records, we still need to delete the secret
	// records.
	q := `
SELECT (r.uuid, r.secret_id) AS (&secretRevision.*)
FROM   secret_application_owner o JOIN secret_revision r ON o.secret_id = r.secret_id
WHERE  application_uuid = $entityUUID.uuid`
	ownedStmt, err := st.Prepare(q, secretRevision{}, appUUID)
	if err != nil {
		return errors.Errorf("preparing secret deletion temp table: %w", err)
	}

	rdStmts, sdStmts, err := st.prepareSecretDeletions()
	if err != nil {
		return errors.Capture(err)
	}

	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var srs secretRevisions
		if err := tx.Query(ctx, ownedStmt, appUUID).GetAll(&srs); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return nil
			}
			return errors.Errorf("running app secret revision query: %w", err)
		}

		rUUIDs, sUUIDs := srs.split()

		for i, stmt := range rdStmts {
			if err := tx.Query(ctx, stmt, rUUIDs).Run(); err != nil {
				return errors.Errorf("running app secret revision deletion statement at index %d: %w", i, err)
			}
		}

		for i, stmt := range sdStmts {
			if err := tx.Query(ctx, stmt, sUUIDs).Run(); err != nil {
				return errors.Errorf("running app secret deletion statement at index %d: %w", i, err)
			}
		}

		return nil
	}); err != nil {
		return errors.Capture(err)
	}

	return nil
}

// DeleteUnitOwnedSecrets deletes all data for secrets owned by the
// unit with the input UUID.
// This does not include secret content, which should be handled by
// interaction with the secret back-end.
func (st *State) DeleteUnitOwnedSecrets(ctx context.Context, uUUID string) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	unitUUID := entityUUID{UUID: uUUID}

	// We need to materialise this up-front, because once we delete the
	// unit_secret_owner records, we still need to delete the secret records.
	q := `
SELECT (r.uuid, r.secret_id) AS (&secretRevision.*)
FROM   secret_unit_owner o JOIN secret_revision r ON o.secret_id = r.secret_id
WHERE  unit_uuid = $entityUUID.uuid`
	ownedStmt, err := st.Prepare(q, secretRevision{}, unitUUID)
	if err != nil {
		return errors.Errorf("preparing secret deletion temp table: %w", err)
	}

	rdStmts, sdStmts, err := st.prepareSecretDeletions()
	if err != nil {
		return errors.Capture(err)
	}

	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var srs secretRevisions
		if err := tx.Query(ctx, ownedStmt, unitUUID).GetAll(&srs); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return nil
			}
			return errors.Errorf("running unit secret revision query: %w", err)
		}

		rUUIDs, sUUIDs := srs.split()

		for i, stmt := range rdStmts {
			if err := tx.Query(ctx, stmt, rUUIDs).Run(); err != nil {
				return errors.Errorf("running unit secret revision deletion statement at index %d: %w", i, err)
			}
		}

		for i, stmt := range sdStmts {
			if err := tx.Query(ctx, stmt, sUUIDs).Run(); err != nil {
				return errors.Errorf("running unit secret deletion statement at index %d: %w", i, err)
			}
		}

		return nil
	}); err != nil {
		return errors.Capture(err)
	}

	return nil
}

func (st *State) prepareSecretDeletions() ([]*sqlair.Statement, []*sqlair.Statement, error) {
	var err error
	dummy := uuids{}

	// Deletions by secret revision UUID.
	rds := []string{
		"DELETE FROM secret_value_ref WHERE revision_uuid IN ($uuids[:])",
		"DELETE FROM secret_deleted_value_ref WHERE revision_uuid IN ($uuids[:])",
		"DELETE FROM secret_revision_obsolete WHERE revision_uuid IN ($uuids[:])",
		"DELETE FROM secret_revision_expire WHERE revision_uuid IN ($uuids[:])",
	}
	rdStmts := make([]*sqlair.Statement, len(rds))
	for i, q := range rds {
		rdStmts[i], err = sqlair.Prepare(q, dummy)
		if err != nil {
			return nil, nil, errors.Capture(err)
		}
	}

	// Deletions by secret UUID.
	sds := []string{
		"DELETE FROM secret_revision WHERE secret_id IN ($uuids[:])",
		"DELETE FROM secret_unit_consumer WHERE secret_id IN ($uuids[:])",
		"DELETE FROM secret_remote_unit_consumer WHERE secret_id IN ($uuids[:])",
		"DELETE FROM secret_rotation WHERE secret_id IN ($uuids[:])",
		"DELETE FROM secret_reference WHERE secret_id IN ($uuids[:])",
		"DELETE FROM secret_permission WHERE secret_id IN ($uuids[:])",
		"DELETE FROM secret_application_owner WHERE secret_id IN ($uuids[:])",
		"DELETE FROM secret_unit_owner WHERE secret_id IN ($uuids[:])",
		"DELETE FROM secret_metadata WHERE secret_id IN ($uuids[:])",
		"DELETE FROM secret WHERE id IN ($uuids[:])",
	}
	sdStmts := make([]*sqlair.Statement, len(sds))
	for i, q := range sds {
		sdStmts[i], err = sqlair.Prepare(q, dummy)
		if err != nil {
			return nil, nil, errors.Capture(err)
		}
	}

	return rdStmts, sdStmts, nil
}

// DeleteUserSecretRevisions deletes the specified revisions of the user secret
// with the input URI. If revisions is nil or empty, all revisions are deleted.
// If the last remaining revisions are removed, the secret is deleted.
// Returns the revision UUIDs that were deleted (for backend cleanup).
func (st *State) DeleteUserSecretRevisions(ctx context.Context, uri *coresecrets.URI, revs []int) ([]string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var deletedUUIDs []string
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		deletedUUIDs, err = st.deleteUserSecretRevisions(ctx, tx, uri, revs)
		if err != nil {
			return errors.Capture(err)
		}
		return nil
	}); err != nil {
		return nil, errors.Capture(err)
	}

	return deletedUUIDs, nil
}

// DeleteObsoleteUserSecretRevisions deletes all obsolete revisions of
// auto-prune user (model-owned) secrets. Returns the deleted revision UUIDs
// for back-end content cleanup.
func (st *State) DeleteObsoleteUserSecretRevisions(ctx context.Context) ([]string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	q := `
SELECT smo.secret_id AS &secretID.secret_id,
       sr.revision AS &secretExternalRevision.revision
FROM      secret_model_owner smo
JOIN      secret_metadata sm ON sm.secret_id = smo.secret_id
JOIN      secret_revision sr ON sr.secret_id = smo.secret_id
LEFT JOIN secret_revision_obsolete sro ON sro.revision_uuid = sr.uuid
WHERE     sm.auto_prune = true AND sro.obsolete = true`

	stmt, err := st.Prepare(q, secretID{}, secretExternalRevision{})
	if err != nil {
		return nil, errors.Capture(err)
	}
	var deletedRevisionIDs []string
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var (
			dbSecrets    secretIDList
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
			deleted, err := st.deleteUserSecretRevisions(ctx, tx, toDelete.URI, revs)
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

func (st *State) deleteUserSecretRevisions(
	ctx context.Context, tx *sqlair.TX, uri *coresecrets.URI, revNums []int,
) ([]string, error) {
	type revisions []int
	type deleteFlags struct {
		HasRevisions int `db:"has_revisions"`
	}

	selectRevsStmt, err := st.Prepare(`
SELECT uuid AS &entityUUID.uuid
FROM   secret_revision r
WHERE  r.secret_id = $secretID.secret_id
AND    ($deleteFlags.has_revisions = 0 OR r.revision IN ($revisions[:]))
  `, secretID{}, deleteFlags{}, revisions{}, entityUUID{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	// Get the revision UUIDs to delete.
	var revUUIDs []entityUUID
	sid := secretID{ID: uri.ID}
	flags := deleteFlags{HasRevisions: len(revNums)}
	err = tx.Query(ctx, selectRevsStmt, sid, flags, revisions(revNums)).GetAll(&revUUIDs)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Errorf("selecting revision UUIDs to delete: %w", err)
	}
	if len(revUUIDs) == 0 {
		// No revisions found - this is OK, secret might already be deleted
		return nil, nil
	}

	// Convert to string slice.
	revUUIDStrs := make(uuids, len(revUUIDs))
	for i, r := range revUUIDs {
		revUUIDStrs[i] = r.UUID
	}

	st.logger.Debugf(ctx, "deleting %d revisions for secret %q", len(revUUIDStrs), uri.String())

	// Delete revision data.
	deleteRevisionQueries := []string{
		`DELETE FROM secret_revision_expire WHERE revision_uuid IN ($uuids[:])`,
		`DELETE FROM secret_content WHERE revision_uuid IN ($uuids[:])`,
		`
INSERT OR IGNORE INTO secret_deleted_value_ref (revision_uuid,backend_uuid,revision_id)
SELECT revision_uuid,backend_uuid,revision_id 
FROM secret_value_ref 
WHERE revision_uuid IN ($uuids[:])`,
		`DELETE FROM secret_value_ref WHERE revision_uuid IN ($uuids[:])`,
		`DELETE FROM secret_revision_obsolete WHERE revision_uuid IN ($uuids[:])`,
		`DELETE FROM secret_revision WHERE uuid IN ($uuids[:])`,
	}

	for _, q := range deleteRevisionQueries {
		stmt, err := st.Prepare(q, revUUIDStrs)
		if err != nil {
			return nil, errors.Capture(err)
		}
		if err := tx.Query(ctx, stmt, revUUIDStrs).Run(); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return nil, errors.Errorf("deleting revision data: %w", err)
		}
	}

	// Check if any revisions remain.
	countRevisions := `SELECT count(*) AS &M.count FROM secret_revision WHERE secret_id = $secretID.secret_id`
	countRevisionsStmt, err := st.Prepare(countRevisions, secretID{}, sqlair.M{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	countResult := sqlair.M{}
	err = tx.Query(ctx, countRevisionsStmt, sid).Get(&countResult)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Errorf("counting remaining revisions: %w", err)
	}

	count := 0
	if countResult != nil && countResult["count"] != nil {
		count, _ = strconv.Atoi(fmt.Sprint(countResult["count"]))
	}

	// If there are still some revisions, leave the secret alone.
	if count > 0 {
		return revUUIDStrs, nil
	}

	// No revisions remain, delete the secret and all related records.
	deleteSecretQueries := []string{
		`DELETE FROM secret_rotation WHERE secret_id = $secretID.secret_id`,
		`DELETE FROM secret_unit_owner WHERE secret_id = $secretID.secret_id`,
		`DELETE FROM secret_application_owner WHERE secret_id = $secretID.secret_id`,
		`DELETE FROM secret_model_owner WHERE secret_id = $secretID.secret_id`,
		`DELETE FROM secret_unit_consumer WHERE secret_id = $secretID.secret_id`,
		`DELETE FROM secret_remote_unit_consumer WHERE secret_id = $secretID.secret_id`,
		`DELETE FROM secret_reference WHERE secret_id = $secretID.secret_id`,
		`DELETE FROM secret_permission WHERE secret_id = $secretID.secret_id`,
		`DELETE FROM secret_metadata WHERE secret_id = $secretID.secret_id`,
		`DELETE FROM secret WHERE id = $secretID.secret_id`,
	}

	for _, q := range deleteSecretQueries {
		stmt, err := st.Prepare(q, sid)
		if err != nil {
			return nil, errors.Capture(err)
		}
		if err := tx.Query(ctx, stmt, sid).Run(); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return nil, errors.Errorf("deleting secret data: %w", err)
		}
	}

	return revUUIDStrs, nil
}

// GetUserSecretRevisionRefs returns the back-end value references for the
// specified revision UUIDs.
func (st *State) GetUserSecretRevisionRefs(ctx context.Context, revs []string) ([]string, error) {
	if len(revs) == 0 {
		return nil, nil
	}

	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	q := `
SELECT revision_id AS &entityUUID.uuid
FROM   secret_deleted_value_ref
WHERE  revision_uuid IN ($uuids[:])`

	stmt, err := st.Prepare(q, uuids{}, entityUUID{})
	if err != nil {
		return nil, errors.Errorf("preparing backend refs query: %w", err)
	}

	var refs []entityUUID
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, uuids(revs)).GetAll(&refs); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("selecting backend refs query: %w", err)
		}
		return nil
	}); err != nil {
		return nil, errors.Capture(err)
	}

	result := make([]string, len(refs))
	for i, r := range refs {
		result[i] = r.UUID
	}
	return result, nil
}

// DeleteUserSecretRevisionRef deletes the back-end value reference for the
// specified deleted revision UUID.
func (st *State) DeleteUserSecretRevisionRef(ctx context.Context, revisionID string) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	stmt, err := st.Prepare(`
DELETE FROM secret_deleted_value_ref
WHERE  revision_id = $secretExternalRevision.revision_id
`, secretExternalRevision{})
	if err != nil {
		return errors.Errorf("preparing deleted value ref deletion query: %w", err)
	}

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		param := secretExternalRevision{RevisionID: revisionID}
		if err := tx.Query(ctx, stmt, param).Run(); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("deleting deleted value ref for revision %q: %w", revisionID, err)
		}
		return nil
	}))
}

func (st *State) deleteOwnedSecretReferences(ctx context.Context, tx *sqlair.TX, appUUID entityUUID) error {
	getConsumedSecretsStmt, err := st.Prepare(`
SELECT &secretID.*
FROM   secret_reference
WHERE  owner_application_uuid = $entityUUID.uuid
`, secretID{}, appUUID)
	if err != nil {
		return errors.Errorf("preparing consumed secrets query: %w", err)
	}

	deleteSecretUnitConsumerStmt, err := st.Prepare(`
DELETE FROM secret_unit_consumer
WHERE  secret_id IN ($secretIDs[:])
`, secretIDs{})
	if err != nil {
		return errors.Errorf("preparing delete secret unit consumers query: %w", err)
	}

	deleteSecretReferencesStmt, err := st.Prepare(`
DELETE FROM secret_reference
WHERE  secret_id IN ($secretIDs[:])
`, secretIDs{})
	if err != nil {
		return errors.Errorf("preparing delete secret references query: %w", err)
	}

	deleteSecretStmt, err := st.Prepare(`
DELETE FROM secret
WHERE  id IN ($secretIDs[:])
`, secretIDs{})
	if err != nil {
		return errors.Errorf("preparing delete secrets query: %w", err)
	}

	var ids []secretID
	if err := tx.Query(ctx, getConsumedSecretsStmt, appUUID).GetAll(&ids); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("getting consumed secrets: %w", err)
	}
	if len(ids) == 0 {
		return nil
	}
	secretIDs := secretIDs(transform.Slice(ids, func(sid secretID) string { return sid.ID }))

	if err := tx.Query(ctx, deleteSecretUnitConsumerStmt, secretIDs).Run(); err != nil {
		return errors.Errorf("deleting consumed secret unit consumers: %w", err)
	}

	if err := tx.Query(ctx, deleteSecretReferencesStmt, secretIDs).Run(); err != nil {
		return errors.Errorf("deleting consumed secret references: %w", err)
	}

	if err := tx.Query(ctx, deleteSecretStmt, secretIDs).Run(); err != nil {
		return errors.Errorf("deleting consumed secrets: %w", err)
	}

	return nil
}
