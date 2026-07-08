// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	corelease "github.com/juju/juju/core/lease"
	"github.com/juju/juju/internal/errors"
)

// DeleteLeadershipForModel deletes all application-leadership leases for the
// given model. Idempotent: returns nil if no matching leases exist.
func (s *State) DeleteLeadershipForModel(ctx context.Context, modelUUID string) error {
	db, err := s.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	lease := Lease{
		Type:      corelease.ApplicationLeadershipNamespace,
		ModelUUID: modelUUID,
	}

	stmt, err := s.Prepare(`
DELETE FROM lease
WHERE uuid IN (
    SELECT l.uuid FROM lease AS l
    JOIN   lease_type AS t ON l.lease_type_id = t.id
    WHERE  t.type = $Lease.type
    AND    l.model_uuid = $Lease.model_uuid
)`, lease)
	if err != nil {
		return errors.Errorf("preparing delete leadership leases statement: %w", err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return errors.Capture(tx.Query(ctx, stmt, lease).Run())
	})
}
