// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/internal/errors"
)

// DeleteOfferAccess removes the permissions granted for the given offer.
func (st *State) DeleteOfferAccess(ctx context.Context, oUUID string) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	offerUUID := entityUUID{UUID: oUUID}
	stmt, err := st.Prepare(`
DELETE FROM permission
WHERE grant_on = $entityUUID.uuid
`, offerUUID)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, offerUUID).Run(); err != nil {
			return errors.Errorf("deleting offer access: %w", err)
		}
		return nil
	})
	return errors.Capture(err)
}
