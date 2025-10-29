// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"

	"github.com/canonical/sqlair"

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
