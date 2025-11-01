// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/collections/transform"
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
