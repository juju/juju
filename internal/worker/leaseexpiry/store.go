// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leaseexpiry

import (
	"context"
	"database/sql"

	"github.com/juju/errors"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/database"
)

type noRetryRunner interface {
	StdTxnNoRetry(context.Context, func(context.Context, *sql.Tx) error) error
}

// expiryStore deliberately uses StdTxnNoRetry instead of TxnRunner.StdTxn.
// This is because the worker runs on every controller, and we don't want retry
// cascades in the event that the workers are interleaving.
//
// The worker runs on all controllers, and not just the one with the singular
// worker lease, because if that controller is deleted from a cluster we can
// become wedged. The lease expiry worker stops running, so there's no expiry,
// so no other controller can run the singular workers.
type expiryStore struct {
	db     noRetryRunner
	logger logger.Logger
}

// ExpireLeases deletes all unpinned leases that have expired.
func (s *expiryStore) ExpireLeases(ctx context.Context) error {
	err := s.db.StdTxnNoRetry(ctx, func(ctx context.Context, tx *sql.Tx) error {
		// This is split into two queries to avoid a write transaction preventing
		// other writers from writing to the database when there is no work.
		var count int
		err := tx.QueryRowContext(ctx, `
SELECT COUNT(*) AS count
FROM lease AS l
LEFT JOIN lease_pin AS p ON l.uuid = p.lease_uuid
WHERE p.uuid IS NULL
AND l.expiry < datetime('now')`).Scan(&count)
		if err != nil {
			return errors.Trace(err)
		}
		if count == 0 {
			return nil
		}

		result, err := tx.ExecContext(ctx, `
DELETE FROM lease
WHERE uuid IN (
    SELECT l.uuid
    FROM lease AS l
    LEFT JOIN lease_pin AS p ON l.uuid = p.lease_uuid
    WHERE p.uuid IS NULL
    AND l.expiry < datetime('now')
)`)
		if err != nil {
			return errors.Trace(err)
		}

		expired, err := result.RowsAffected()
		if err != nil {
			return errors.Trace(err)
		}
		if expired > 0 {
			s.logger.Infof(ctx, "expired %d leases", expired)
		}
		return nil
	})
	if database.IsErrRetryable(err) {
		s.logger.Debugf(ctx, "ignoring error during lease expiry: %s", err.Error())
		return nil
	}
	return errors.Trace(err)
}
