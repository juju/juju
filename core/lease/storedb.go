// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import (
	"context"
	"database/sql"
	"fmt"
	"math"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/utils/v3"
	"github.com/mattn/go-sqlite3"
)

// StoreLogger describes methods for logging lease store concerns.
type StoreLogger interface {
	Errorf(string, ...interface{})
}

// DBStore implements lease.Store using a database
// supporting SQLite-compatible dialects.
type DBStore struct {
	db     *sql.DB
	logger StoreLogger
}

func NewDBStore(db *sql.DB, logger StoreLogger) *DBStore {
	return &DBStore{
		db:     db,
		logger: logger,
	}
}

// Leases (lease.Store) returns all leases in the database,
// optionally filtering using the input keys.
func (s *DBStore) Leases(keys ...Key) (map[Key]Info, error) {
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

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, errors.Trace(err)
	}

	result, err := leasesFromRows(rows)

	// TODO (manadart 2022-11-30): We expect the variadic `keys` argument to be
	// length 0 or 1. It was a work-around for design constraints at the time
	// that it was instituted. Either filter the result here for len(keys) > 1,
	// or fix the design.

	return result, errors.Trace(rows.Err())
}

// ClaimLease (lease.Store) claims the lease indicated by the input key,
// for the holder indicated by the input duration.
// The lease must not already be held, otherwise an error is returned.
func (s *DBStore) ClaimLease(lease Key, request Request, stop <-chan struct{}) error {
	if err := request.Validate(); err != nil {
		return errors.Trace(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error)

	go func() {
		q := `
INSERT INTO lease (uuid, lease_type_id, model_uuid, name, holder, start, expiry)
SELECT ?, id, ?, ?, ?, datetime('now'), datetime('now', ?) 
FROM   lease_type
WHERE  type = ?`[1:]

		d := fmt.Sprintf("+%d seconds", int64(math.Ceil(request.Duration.Seconds())))

		_, err := s.db.ExecContext(
			ctx, q, utils.MustNewUUID().String(), lease.ModelUUID, lease.Lease, request.Holder, d, lease.Namespace)

		errCh <- err
	}()

	select {
	case <-stop:
		cancel()
		return errors.New("claim cancelled")
	case err := <-errCh:
		cancel()
		// TODO (manadart 2022-12-01): Interpret this such that a UK violation means ErrHeld.
		return errors.Trace(err)
	}
}

func (s *DBStore) ExtendLease(lease Key, request Request, stop <-chan struct{}) error {
	panic("implement me")
}

func (s *DBStore) RevokeLease(lease Key, holder string, stop <-chan struct{}) error {
	panic("implement me")
}

// LeaseGroup (lease.Store) returns all leases
// for the input namespace and model.
func (s *DBStore) LeaseGroup(namespace, modelUUID string) (map[Key]Info, error) {
	q := `
SELECT t.type, l.model_uuid, l.name, l.holder, l.expiry
FROM   lease l JOIN lease_type t ON l.lease_type_id = t.id
WHERE  t.type = ?
AND    l.model_uuid = ?`[1:]

	rows, err := s.db.Query(q, namespace, modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	result, err := leasesFromRows(rows)
	return result, errors.Trace(err)
}

// PinLease (lease.Store) adds the input entity into the lease_pin table
// to indicate that the lease indicated by the input key must not expire,
// and that this entity requires such behaviour.
func (s *DBStore) PinLease(lease Key, entity string, stop <-chan struct{}) error {
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error)

	go func() {
		q := `
INSERT INTO lease_pin (uuid, lease_uuid, entity_id)
SELECT ?, l.uuid, ?
FROM   lease l JOIN lease_type t ON l.lease_type_id = t.id
WHERE  t.type = ?
AND    l.model_uuid = ?
AND    l.name = ?`[1:]

		_, err := s.db.ExecContext(
			ctx, q, utils.MustNewUUID().String(), entity, lease.Namespace, lease.ModelUUID, lease.Lease)

		errCh <- err
	}()

	select {
	case <-stop:
		cancel()
		return errors.New("pin lease cancelled")
	case err := <-errCh:
		cancel()
		if isUniquenessViolation(err) {
			return nil
		}
		return errors.Trace(err)
	}
}

// UnpinLease (lease.Store) removes the record indicated by the input
// key and entity from the lease pin table, indicating that the entity
// no longer requires the lease to be pinned.
// When there are no entities associated with a particular lease,
// it is determined not to be pinned, and can expire normally.
func (s *DBStore) UnpinLease(lease Key, entity string, stop <-chan struct{}) error {
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error)

	go func() {
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
)`[1:]

		_, err := s.db.ExecContext(
			ctx, q, lease.Namespace, lease.ModelUUID, lease.Lease, entity)

		errCh <- err
	}()

	select {
	case <-stop:
		cancel()
		return errors.New("unpin lease cancelled")
	case err := <-errCh:
		cancel()
		return errors.Trace(err)
	}
}

// Pinned (lease.Store) returns all leases that are currently pinned,
// and the entities requiring such behaviour for them.
func (s *DBStore) Pinned() (map[Key][]string, error) {
	q := `
SELECT   l.uuid, t.type, l.model_uuid, l.name, p.entity_id
FROM     lease l 
         JOIN lease_type t ON l.lease_type_id = t.id
         JOIN lease_pin p on l.uuid = p.lease_uuid
ORDER BY l.uuid`[1:]

	rows, err := s.db.Query(q)
	if err != nil {
		return nil, errors.Trace(err)
	}

	seen := set.NewStrings()
	result := make(map[Key][]string)
	for rows.Next() {
		var leaseUUID string
		var key Key
		var entity string

		if err := rows.Scan(&leaseUUID, &key.Namespace, &key.ModelUUID, &key.Lease, &entity); err != nil {
			_ = rows.Close()
			return nil, errors.Trace(err)
		}

		if !seen.Contains(leaseUUID) {
			result[key] = []string{entity}
			seen.Add(leaseUUID)
		} else {
			result[key] = append(result[key], entity)
		}
	}

	return result, errors.Trace(rows.Err())
}

// leasesFromRows returns lease info from rows returned from the backing DB.
func leasesFromRows(rows *sql.Rows) (map[Key]Info, error) {
	result := map[Key]Info{}

	for rows.Next() {
		var key Key
		var info Info

		if err := rows.Scan(&key.Namespace, &key.ModelUUID, &key.Lease, &info.Holder, &info.Expiry); err != nil {
			_ = rows.Close()
			return nil, errors.Trace(err)
		}
		result[key] = info
	}

	return result, errors.Trace(rows.Err())
}

// TODO (manadart 2022-12-05): Utilities like this will reside in
// the database package for general use.
func isUniquenessViolation(err error) bool {
	var sqliteErr sqlite3.Error
	if errors.As(err, &sqliteErr) {
		if errors.Is(sqliteErr.ExtendedCode, sqlite3.ErrConstraintUnique) {
			return true
		}
	}
	return false
}
