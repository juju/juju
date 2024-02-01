// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	"math"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/utils/v3"

	coreDB "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/database/txn"
)

// Logger is the interface used by the state to log messages.
type Logger interface {
	Debugf(string, ...interface{})
	Infof(string, ...interface{})
}

// State describes retrieval and persistence methods for storage.
type State struct {
	*domain.StateBase
	logger Logger
}

// NewState returns a new state reference.
func NewState(factory coreDB.TxnRunnerFactory, logger Logger) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
		logger:    logger,
	}
}

// Leases (lease.Store) returns all leases in the database,
// optionally filtering using the input keys.
func (s *State) Leases(ctx context.Context, keys ...lease.Key) (map[lease.Key]lease.Info, error) {
	// TODO (manadart 2022-11-30): We expect the variadic `keys` argument to be
	// length 0 or 1. It was a work-around for design constraints at the time.
	// Either filter the result here for len(keys) > 1, or fix the design.
	// As it is, there are no upstream usages for more than one key,
	// so we just lock in that behaviour.
	if len(keys) > 1 {
		return nil, errors.NotSupportedf("filtering with more than one lease key")
	}

	db, err := s.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	q := `
SELECT t.type, l.model_uuid, l.name, l.holder, l.expiry
FROM   lease l JOIN lease_type t ON l.lease_type_id = t.id`[1:]

	var args []any

	if len(keys) == 1 {
		q += `
WHERE  t.type = ?
AND    l.model_uuid = ?
AND    l.name = ?`

		key := keys[0]
		args = []any{key.Namespace, key.ModelUUID, key.Lease}
	}

	var result map[lease.Key]lease.Info
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, q, args...)
		if err != nil {
			return errors.Trace(err)
		}

		result, err = leasesFromRows(rows)
		return errors.Trace(err)
	})
	return result, errors.Trace(err)
}

// ClaimLease (lease.Store) claims the lease indicated by the input key,
// for the holder and duration indicated by the input request.
// The lease must not already be held, otherwise an error is returned.
func (s *State) ClaimLease(ctx context.Context, uuid utils.UUID, key lease.Key, req lease.Request) error {
	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
	}

	q := `
INSERT INTO lease (uuid, lease_type_id, model_uuid, name, holder, start, expiry)
SELECT ?, id, ?, ?, ?, datetime('now'), datetime('now', ?) 
FROM   lease_type
WHERE  type = ?;`[1:]

	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		d := fmt.Sprintf("+%d seconds", int64(math.Ceil(req.Duration.Seconds())))

		_, err := tx.ExecContext(ctx, q, uuid.String(), key.ModelUUID, key.Lease, req.Holder, d, key.Namespace)
		return errors.Trace(err)
	})
	if database.IsErrConstraintUnique(err) {
		return lease.ErrHeld
	}
	return errors.Trace(err)
}

// ExtendLease (lease.Store) ensures the input lease will be held for at least
// the requested duration starting from now.
// If the input holder does not currently hold the lease, an error is returned.
func (s *State) ExtendLease(ctx context.Context, key lease.Key, req lease.Request) error {
	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
	}

	q := `
UPDATE lease
SET    expiry = datetime('now', ?)
WHERE  uuid = (
    SELECT l.uuid
    FROM   lease l JOIN lease_type t ON l.lease_type_id = t.id
    WHERE  t.type = ?
    AND    l.model_uuid = ?
    AND    l.name = ?
    AND    l.holder = ?
)`[1:]

	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		d := fmt.Sprintf("+%d seconds", int64(math.Ceil(req.Duration.Seconds())))
		result, err := tx.ExecContext(ctx, q, d, key.Namespace, key.ModelUUID, key.Lease, req.Holder)

		// If no rows were affected, then either this key does not exist or
		// it is not held by the input holder, constituting an invalid request.
		if err == nil {
			var affected int64
			affected, err = result.RowsAffected()
			if affected == 0 && err == nil {
				err = lease.ErrInvalid
			}
		}
		return errors.Trace(err)
	})
	return errors.Trace(err)
}

// RevokeLease (lease.Store) deletes the lease from the store,
// provided it exists and is held by the input holder.
// If either of these conditions is false, an error is returned.
func (s *State) RevokeLease(ctx context.Context, key lease.Key, holder string) error {
	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
	}

	q := `
DELETE FROM lease
WHERE  uuid = (
    SELECT l.uuid
    FROM   lease l JOIN lease_type t ON l.lease_type_id = t.id
    WHERE  t.type = ?
    AND    l.model_uuid = ?
    AND    l.name = ?
    AND    l.holder = ?
);`[1:]

	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {

		result, err := tx.ExecContext(ctx, q, key.Namespace, key.ModelUUID, key.Lease, holder)
		if err == nil {
			var affected int64
			affected, err = result.RowsAffected()
			if affected == 0 && err == nil {
				err = lease.ErrInvalid
			}
		}
		return errors.Trace(err)
	})
	return errors.Trace(err)
}

// LeaseGroup (lease.Store) returns all leases
// for the input namespace and model.
func (s *State) LeaseGroup(ctx context.Context, namespace, modelUUID string) (map[lease.Key]lease.Info, error) {
	db, err := s.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	q := `
SELECT t.type, l.model_uuid, l.name, l.holder, l.expiry
FROM   lease l JOIN lease_type t ON l.lease_type_id = t.id
WHERE  t.type = ?
AND    l.model_uuid = ?;`[1:]

	var result map[lease.Key]lease.Info
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, q, namespace, modelUUID)
		if err != nil {
			return errors.Trace(err)
		}

		result, err = leasesFromRows(rows)
		return errors.Trace(err)
	})
	return result, errors.Trace(err)
}

// PinLease (lease.Store) adds the input entity into the lease_pin table
// to indicate that the lease indicated by the input key must not expire,
// and that this entity requires such behaviour.
func (s *State) PinLease(ctx context.Context, key lease.Key, entity string) error {
	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
	}

	q := `
INSERT INTO lease_pin (uuid, lease_uuid, entity_id)
SELECT ?, l.uuid, ?
FROM   lease l JOIN lease_type t ON l.lease_type_id = t.id
WHERE  t.type = ?
AND    l.model_uuid = ?
AND    l.name = ?;`[1:]

	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, q, utils.MustNewUUID().String(), entity, key.Namespace, key.ModelUUID, key.Lease)
		return errors.Trace(err)
	})
	if database.IsErrConstraintUnique(err) {
		return nil
	}
	return errors.Trace(err)
}

// UnpinLease (lease.Store) removes the record indicated by the input
// key and entity from the lease pin table, indicating that the entity
// no longer requires the lease to be pinned.
// When there are no entities associated with a particular lease,
// it is determined not to be pinned, and can expire normally.
func (s *State) UnpinLease(ctx context.Context, key lease.Key, entity string) error {
	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
	}

	q := `
DELETE FROM lease_pin
WHERE  uuid = (
    SELECT p.uuid
    FROM   lease_pin p
           JOIN lease l ON l.uuid = p.lease_uuid
           JOIN lease_type t ON l.lease_type_id = t.id
    WHERE  t.type = ?
    AND    l.model_uuid = ?
    AND    l.name = ?
    AND    p.entity_id = ?   
);`[1:]
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, q, key.Namespace, key.ModelUUID, key.Lease, entity)
		return errors.Trace(err)
	})
	return errors.Trace(err)
}

// Pinned (lease.Store) returns all leases that are currently pinned,
// and the entities requiring such behaviour for them.
func (s *State) Pinned(ctx context.Context) (map[lease.Key][]string, error) {
	db, err := s.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	q := `
SELECT   l.uuid, t.type, l.model_uuid, l.name, p.entity_id
FROM     lease l 
         JOIN lease_type t ON l.lease_type_id = t.id
		 JOIN lease_pin p on l.uuid = p.lease_uuid
ORDER BY l.uuid;`[1:]

	var result map[lease.Key][]string
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, q)
		if err != nil {
			return errors.Trace(err)
		}
		defer rows.Close()

		seen := set.NewStrings()

		result = make(map[lease.Key][]string)
		for rows.Next() {
			var leaseUUID string
			var key lease.Key
			var entity string

			if err := rows.Scan(&leaseUUID, &key.Namespace, &key.ModelUUID, &key.Lease, &entity); err != nil {
				return errors.Trace(err)
			}

			if !seen.Contains(leaseUUID) {
				result[key] = []string{entity}
				seen.Add(leaseUUID)
			} else {
				result[key] = append(result[key], entity)
			}
		}
		return errors.Trace(rows.Err())
	})
	return result, errors.Trace(err)
}

// ExpireLeases (lease.Store) deletes all leases that have expired, from the
// store. This method is intended to be called periodically by a worker.
func (s *State) ExpireLeases(ctx context.Context) error {
	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
	}

	// This is split into two queries to avoid a write transaction preventing
	// other writers from writing to the db, even if there is no writes
	// occurring.
	countQuery := `
SELECT COUNT(*) FROM lease WHERE expiry < datetime('now');
`

	deleteQuery := `
DELETE FROM lease WHERE uuid in (
	SELECT l.uuid 
	FROM   lease l LEFT JOIN lease_pin p ON l.uuid = p.lease_uuid
	WHERE  p.uuid IS NULL
	AND    l.expiry < datetime('now')
);`

	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		var count int64
		row := tx.QueryRowContext(ctx, countQuery)
		if err := row.Scan(&count); err != nil {
			if txn.IsErrRetryable(err) {
				return nil
			}
			return errors.Trace(err)
		}

		// Nothing to do here, so return early.
		if count == 0 {
			return nil
		}

		res, err := tx.ExecContext(ctx, deleteQuery)
		if err != nil {
			// TODO (manadart 2022-12-15): This incarnation of the worker runs on
			// all controller nodes. Retryable errors are those that occur due to
			// locking or other contention. We know we will retry very soon,
			// so just log and indicate success for these cases.
			// Rethink this if the worker cardinality changes to be singular.
			if txn.IsErrRetryable(err) {
				s.logger.Debugf("ignoring error during lease expiry: %s", err.Error())
				return nil
			}
			return errors.Trace(err)
		}

		expired, err := res.RowsAffected()
		if err != nil {
			return errors.Trace(err)
		}

		if expired > 0 {
			s.logger.Infof("expired %d leases", expired)
		}

		return nil
	})
	return errors.Trace(err)
}

// leasesFromRows returns lease info from rows returned from the backing DB.
func leasesFromRows(rows *sql.Rows) (map[lease.Key]lease.Info, error) {
	result := map[lease.Key]lease.Info{}

	for rows.Next() {
		var key lease.Key
		var info lease.Info

		if err := rows.Scan(&key.Namespace, &key.ModelUUID, &key.Lease, &info.Holder, &info.Expiry); err != nil {
			_ = rows.Close()
			return nil, errors.Trace(err)
		}
		result[key] = info
	}

	return result, errors.Trace(rows.Err())
}
