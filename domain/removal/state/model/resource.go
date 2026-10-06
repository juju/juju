// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/internal/errors"
)

// ResourceExists reports whether the resource exists.
func (st *State) ResourceExists(ctx context.Context, resourceUUID string) (bool, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return false, errors.Capture(err)
	}

	resourceID := entityUUID{UUID: resourceUUID}
	stmt, err := st.Prepare(`
SELECT uuid AS &entityUUID.uuid
FROM   resource
WHERE  uuid = $entityUUID.uuid
`, resourceID)
	if err != nil {
		return false, errors.Capture(err)
	}

	exists := false
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists = false
		var resource entityUUID
		err := tx.Query(ctx, stmt, resourceID).Get(&resource)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return errors.Capture(err)
		}
		exists = true
		return nil
	})
	return exists, errors.Capture(err)
}

// DeleteResourceIfUnused deletes a resource if it has no application,
// pending application, or unit references. It returns true when the resource
// was deleted or was already absent, and false when it is still referenced.
func (st *State) DeleteResourceIfUnused(ctx context.Context, resourceUUID string) (bool, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return false, errors.Capture(err)
	}

	resourceID := entityUUID{UUID: resourceUUID}
	selectUnusedStmt, err := st.Prepare(`
WITH referenced_resource AS (
    SELECT ar.resource_uuid AS uuid
    FROM   application_resource AS ar
    WHERE  ar.resource_uuid = $entityUUID.uuid
    UNION
    SELECT par.resource_uuid AS uuid
    FROM   pending_application_resource AS par
    WHERE  par.resource_uuid = $entityUUID.uuid
    UNION
    SELECT ur.resource_uuid AS uuid
    FROM   unit_resource AS ur
    WHERE  ur.resource_uuid = $entityUUID.uuid
)
SELECT r.uuid AS &entityUUID.uuid
FROM   resource AS r
LEFT JOIN referenced_resource AS rr ON rr.uuid = r.uuid
WHERE  r.uuid = $entityUUID.uuid
AND    rr.uuid IS NULL
`, resourceID)
	if err != nil {
		return false, errors.Capture(err)
	}

	resourceExistsStmt, err := st.Prepare(`
SELECT uuid AS &entityUUID.uuid
FROM   resource
WHERE  uuid = $entityUUID.uuid
`, resourceID)
	if err != nil {
		return false, errors.Capture(err)
	}

	removed := false
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		removed = false
		var unusedResource entityUUID
		err := tx.Query(ctx, selectUnusedStmt, resourceID).Get(&unusedResource)
		if errors.Is(err, sqlair.ErrNoRows) {
			var existingResource entityUUID
			err = tx.Query(ctx, resourceExistsStmt, resourceID).Get(&existingResource)
			if errors.Is(err, sqlair.ErrNoRows) {
				removed = true
				return nil
			}
			return errors.Capture(err)
		} else if err != nil {
			return errors.Capture(err)
		}

		if err := st.deleteResources(ctx, tx, []entityUUID{unusedResource}); err != nil {
			return errors.Capture(err)
		}
		removed = true
		return nil
	})
	return removed, errors.Capture(err)
}
