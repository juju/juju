// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"

	coredatabase "github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	corelease "github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// State describes retrieval and persistence methods for storage.
type State struct {
	*domain.StateBase
	logger logger.Logger
}

// NewState returns a new state reference.
func NewState(factory coredatabase.TxnRunnerFactory, logger logger.Logger) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
		logger:    logger,
	}
}

// Leases (lease.Store) returns all leases in the database,
// optionally filtering using the input keys.
func (s *State) Leases(ctx context.Context, keys ...corelease.Key) (map[corelease.Key]corelease.Info, error) {
	// TODO (manadart 2022-11-30): We expect the variadic `keys` argument to be
	// length 0 or 1. It was a work-around for design constraints at the time.
	// Either filter the result here for len(keys) > 1, or fix the design.
	// As it is, there are no upstream usages for more than one key,
	// so we just lock in that behaviour.
	if len(keys) > 1 {
		return nil, errors.Errorf("filtering with more than one lease key %w", coreerrors.NotSupported)
	}

	db, err := s.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	q := `
SELECT (t.type, l.model_uuid, l.name, l.holder, l.expiry) AS (&Lease.*)
FROM   lease l JOIN lease_type t ON l.lease_type_id = t.id`

	var args []any

	var stmt *sqlair.Statement
	if len(keys) == 1 {
		key := keys[0]
		lease := Lease{
			Type:      key.Namespace,
			ModelUUID: key.ModelUUID,
			Name:      key.Lease,
		}

		stmt, err = s.Prepare(q+`
WHERE  t.type = $Lease.type
AND    l.model_uuid = $Lease.model_uuid
AND    l.name = $Lease.name`, lease)
		if err != nil {
			return nil, errors.Errorf("preparing select lease with keys statement: %w", err)
		}

		args = []any{lease}
	} else {
		stmt, err = s.Prepare(q, Lease{})
		if err != nil {
			return nil, errors.Errorf("preparing select lease statement: %w", err)
		}
	}

	var result map[corelease.Key]corelease.Info
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var leases []Lease
		err := tx.Query(ctx, stmt, args...).GetAll(&leases)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return errors.Capture(err)
		}

		result = map[corelease.Key]corelease.Info{}
		for _, lease := range leases {
			result[corelease.Key{
				Namespace: lease.Type,
				ModelUUID: lease.ModelUUID,
				Lease:     lease.Name,
			}] = corelease.Info{
				Holder: lease.Holder,
				Expiry: lease.Expiry,
			}
		}
		return nil
	})
	return result, errors.Capture(err)
}

// ClaimLease (lease.Store) claims the lease indicated by the input key,
// for the holder and duration indicated by the input request.
// The lease must not already be held, otherwise an error is returned.
func (s *State) ClaimLease(ctx context.Context, uuid uuid.UUID, key corelease.Key, req corelease.Request) error {
	db, err := s.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	lease := Lease{
		UUID:      uuid.String(),
		ModelUUID: key.ModelUUID,
		Type:      key.Namespace,
		Name:      key.Lease,
		Holder:    req.Holder,
		Duration:  LeaseDuration(req.Duration),
	}

	stmt, err := s.Prepare(`
INSERT INTO lease (uuid, lease_type_id, model_uuid, name, holder, start, expiry)
SELECT $Lease.uuid, id, $Lease.model_uuid, $Lease.name, $Lease.holder, datetime('now'), datetime('now', $Lease.duration) 
FROM   lease_type
WHERE  type = $Lease.type;`, lease)
	if err != nil {
		return errors.Errorf("preparing insert lease statement: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, lease).Run()
		if database.IsErrConstraintUnique(err) {
			return corelease.ErrHeld
		} else if err != nil {
			return errors.Capture(err)
		}
		return nil
	})

	return errors.Capture(err)
}

// ExtendLease (lease.Store) ensures the input lease will be held for at least
// the requested duration starting from now.
// If the input holder does not currently hold the lease, an error is returned.
func (s *State) ExtendLease(ctx context.Context, key corelease.Key, req corelease.Request) error {
	db, err := s.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	lease := Lease{
		Duration:  LeaseDuration(req.Duration),
		ModelUUID: key.ModelUUID,
		Type:      key.Namespace,
		Name:      key.Lease,
		Holder:    req.Holder,
	}

	stmt, err := s.Prepare(`
UPDATE lease
SET    expiry = datetime('now', $Lease.duration)
WHERE  uuid = (
    SELECT l.uuid
    FROM   lease l JOIN lease_type t ON l.lease_type_id = t.id
    WHERE  t.type = $Lease.type
    AND    l.model_uuid = $Lease.model_uuid
    AND    l.name = $Lease.name
    AND    l.holder = $Lease.holder
)`, lease)
	if err != nil {
		return errors.Errorf("preparing update lease statement: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var outcome sqlair.Outcome
		err := tx.Query(ctx, stmt, lease).Get(&outcome)

		// If no rows were affected, then either this key does not exist or
		// it is not held by the input holder, constituting an invalid request.
		if err == nil {
			var affected int64
			affected, err = outcome.Result().RowsAffected()
			if affected == 0 && err == nil {
				err = corelease.ErrInvalid
			}
		}
		return errors.Capture(err)
	})
	return errors.Capture(err)
}

// RevokeLease (lease.Store) deletes the lease from the store,
// provided it exists and is held by the input holder.
// If either of these conditions is false, an error is returned.
func (s *State) RevokeLease(ctx context.Context, key corelease.Key, holder string) error {
	db, err := s.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	lease := Lease{
		ModelUUID: key.ModelUUID,
		Name:      key.Lease,
		Holder:    holder,
		Type:      key.Namespace,
	}

	stmt, err := s.Prepare(`
DELETE FROM lease
WHERE  uuid = (
    SELECT l.uuid
    FROM   lease l JOIN lease_type t ON l.lease_type_id = t.id
    WHERE  t.type = $Lease.type
    AND    l.model_uuid = $Lease.model_uuid
    AND    l.name = $Lease.name
    AND    l.holder = $Lease.holder
)`, lease)
	if err != nil {
		return errors.Errorf("preparing delete lease statement: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var outcome sqlair.Outcome
		err := tx.Query(ctx, stmt, lease).Get(&outcome)
		if err == nil {
			var affected int64
			affected, err = outcome.Result().RowsAffected()
			if affected == 0 && err == nil {
				err = corelease.ErrInvalid
			}
		}
		return errors.Capture(err)
	})
	return errors.Capture(err)
}

// LeaseGroup (lease.Store) returns all leases
// for the input namespace and model.
func (s *State) LeaseGroup(ctx context.Context, namespace, modelUUID string) (map[corelease.Key]corelease.Info, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	lease := Lease{
		ModelUUID: modelUUID,
		Type:      namespace,
	}

	stmt, err := s.Prepare(`
SELECT (t.type, l.model_uuid, l.name, l.holder, l.expiry) AS (&Lease.*)
FROM   lease l JOIN lease_type t ON l.lease_type_id = t.id
WHERE  t.type = $Lease.type
AND    l.model_uuid = $Lease.model_uuid;`, lease)
	if err != nil {
		return nil, errors.Errorf("preparing delete lease statement: %w", err)
	}

	var result map[corelease.Key]corelease.Info
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var leases []Lease
		err := tx.Query(ctx, stmt, lease).GetAll(&leases)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return errors.Capture(err)
		}

		result = map[corelease.Key]corelease.Info{}
		for _, lease := range leases {
			result[corelease.Key{
				Namespace: lease.Type,
				ModelUUID: lease.ModelUUID,
				Lease:     lease.Name,
			}] = corelease.Info{
				Holder: lease.Holder,
				Expiry: lease.Expiry,
			}
		}
		return nil
	})
	return result, errors.Capture(err)
}

// PinLease (lease.Store) adds the input entity into the lease_pin table
// to indicate that the lease indicated by the input key must not expire,
// and that this entity requires such behaviour.
func (s *State) PinLease(ctx context.Context, key corelease.Key, entity string) error {
	db, err := s.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	uuid, err := uuid.NewUUID()
	if err != nil {
		return errors.Capture(err)
	}

	leasePin := LeasePin{
		UUID:     uuid.String(),
		EntityID: entity,
	}
	lease := Lease{
		Type:      key.Namespace,
		ModelUUID: key.ModelUUID,
		Name:      key.Lease,
	}

	stmt, err := s.Prepare(`
INSERT INTO lease_pin (uuid, lease_uuid, entity_id)
SELECT $LeasePin.uuid, l.uuid, $LeasePin.entity_id
FROM   lease l JOIN lease_type t ON l.lease_type_id = t.id
WHERE  t.type = $Lease.type
AND    l.model_uuid = $Lease.model_uuid
AND    l.name = $Lease.name;`, leasePin, lease)
	if err != nil {
		return errors.Errorf("preparing insert lease pin statement: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, leasePin, lease).Run()
		if database.IsErrConstraintUnique(err) {
			return nil
		} else if err != nil {
			return errors.Capture(err)
		}
		return nil
	})

	return errors.Capture(err)
}

// UnpinLease (lease.Store) removes the record indicated by the input
// key and entity from the lease pin table, indicating that the entity
// no longer requires the lease to be pinned.
// When there are no entities associated with a particular lease,
// it is determined not to be pinned, and can expire normally.
func (s *State) UnpinLease(ctx context.Context, key corelease.Key, entity string) error {
	db, err := s.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	leasePin := LeasePin{
		EntityID: entity,
	}
	lease := Lease{
		Type:      key.Namespace,
		ModelUUID: key.ModelUUID,
		Name:      key.Lease,
	}

	stmt, err := s.Prepare(`
DELETE FROM lease_pin
WHERE  uuid = (
    SELECT p.uuid
    FROM   lease_pin p
           JOIN lease l ON l.uuid = p.lease_uuid
           JOIN lease_type t ON l.lease_type_id = t.id
    WHERE  t.type = $Lease.type
    AND    l.model_uuid = $Lease.model_uuid
    AND    l.name = $Lease.name
    AND    p.entity_id = $LeasePin.entity_id)`, lease, leasePin)
	if err != nil {
		return errors.Errorf("preparing delete lease pin statement: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return errors.Capture(tx.Query(ctx, stmt, lease, leasePin).Run())
	})
	return errors.Capture(err)
}

// Pinned (lease.Store) returns all leases that are currently pinned,
// and the entities requiring such behaviour for them.
func (s *State) Pinned(ctx context.Context) (map[corelease.Key][]string, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := s.Prepare(`
SELECT   (l.uuid, t.type, l.model_uuid, l.name) AS (&Lease.*),
         (p.entity_id) AS (&LeasePin.*)
FROM     lease l 
         JOIN lease_type t ON l.lease_type_id = t.id
		 JOIN lease_pin p on l.uuid = p.lease_uuid
ORDER BY l.uuid;`, Lease{}, LeasePin{})
	if err != nil {
		return nil, errors.Errorf("preparing select pinned lease statement: %w", err)
	}

	var result map[corelease.Key][]string
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var leases []Lease
		var leasePins []LeasePin
		err := tx.Query(ctx, stmt).GetAll(&leases, &leasePins)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return errors.Capture(err)
		}

		seen := set.NewStrings()

		result = make(map[corelease.Key][]string)
		for i, lease := range leases {
			key := corelease.Key{
				Namespace: lease.Type,
				ModelUUID: lease.ModelUUID,
				Lease:     lease.Name,
			}
			entity := leasePins[i].EntityID

			if !seen.Contains(lease.UUID) {
				result[key] = []string{entity}
				seen.Add(lease.UUID)
			} else {
				result[key] = append(result[key], entity)
			}
		}
		return nil
	})
	return result, errors.Capture(err)
}

// ExpireLeases (lease.Store) deletes all leases that have expired, from the
// store. This method is intended to be called periodically by a worker.
func (s *State) ExpireLeases(ctx context.Context) error {
	db, err := s.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	// This is split into two queries to avoid a write transaction preventing
	// other writers from writing to the db, even if there is no writes
	// occurring.
	count := Count{}
	countStmt, err := s.Prepare(`
SELECT COUNT(*) AS &Count.num FROM lease WHERE expiry < datetime('now');
`, count)
	if err != nil {
		return errors.Errorf("preparing select expired count statement: %w", err)
	}

	deleteStmt, err := s.Prepare(`
DELETE FROM lease WHERE uuid in (
	SELECT l.uuid 
	FROM   lease l LEFT JOIN lease_pin p ON l.uuid = p.lease_uuid
	WHERE  p.uuid IS NULL
	AND    l.expiry < datetime('now')
);`)
	if err != nil {
		return errors.Errorf("preparing delete lease statement: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, countStmt).Get(&count)
		if database.IsErrRetryable(err) {
			return nil
		} else if err != nil {
			return errors.Capture(err)
		}

		// Nothing to do here, so return early.
		if count.Num == 0 {
			return nil
		}

		var outcome sqlair.Outcome
		err = tx.Query(ctx, deleteStmt).Get(&outcome)
		if err != nil {
			// TODO (manadart 2022-12-15): This incarnation of the worker runs on
			// all controller nodes. Retryable errors are those that occur due to
			// locking or other contention. We know we will retry very soon,
			// so just log and indicate success for these cases.
			// Rethink this if the worker cardinality changes to be singular.
			if database.IsErrRetryable(err) {
				s.logger.Debugf(ctx, "ignoring error during lease expiry: %s", err.Error())
				return nil
			}
			return errors.Capture(err)
		}

		expired, err := outcome.Result().RowsAffected()
		if err != nil {
			return errors.Capture(err)
		}

		if expired > 0 {
			s.logger.Infof(ctx, "expired %d leases", expired)
		}

		return nil
	})
	return errors.Capture(err)
}
