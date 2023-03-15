// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"sync"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/utils/v3"

	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/database"
)

// StoreLogger describes methods for logging lease store concerns.
type StoreLogger interface {
	Errorf(string, ...interface{})
}

// StoreConfig encapsulates data required to construct a lease store instance.
type StoreConfig struct {
	// DB is the SQL database that backs this lease store.
	DB *sql.DB

	// Logger is used to emit store-specific diagnostics.
	Logger StoreLogger
}

// Store implements lease.Store using a database
// supporting SQLite-compatible dialects.
type Store struct {
	db     *sql.DB
	logger StoreLogger

	cache   map[string]*sql.Stmt
	cacheMu sync.RWMutex
}

// NewStore returns a reference to a new database-backed lease store.
func NewStore(cfg StoreConfig) *Store {
	return &Store{
		db:     cfg.DB,
		logger: cfg.Logger,
		cache:  make(map[string]*sql.Stmt),
	}
}

// Leases (lease.Store) returns all leases in the database,
// optionally filtering using the input keys.
func (s *Store) Leases(ctx context.Context, keys ...lease.Key) (map[lease.Key]lease.Info, error) {
	// TODO (manadart 2022-11-30): We expect the variadic `keys` argument to be
	// length 0 or 1. It was a work-around for design constraints at the time.
	// Either filter the result here for len(keys) > 1, or fix the design.
	// As it is, there are no upstream usages for more than one key,
	// so we just lock in that behaviour.
	if len(keys) > 1 {
		return nil, errors.NotSupportedf("filtering with more than one lease key")
	}

	name := "Leases"
	q := `
SELECT t.type, l.model_uuid, l.name, l.holder, l.expiry
FROM   lease l JOIN lease_type t ON l.lease_type_id = t.id`[1:]

	var args []any

	if len(keys) == 1 {
		q += `
WHERE  t.type = ?
AND    l.model_uuid = ?
AND    l.name = ?`

		name = "LeasesForKey"
		key := keys[0]
		args = []any{key.Namespace, key.ModelUUID, key.Lease}
	}

	stmt, err := s.getPrepared(ctx, name, q)
	if err != nil {
		return nil, errors.Trace(err)
	}

	rows, err := stmt.QueryContext(ctx, args...)
	if err != nil {
		return nil, errors.Trace(err)
	}

	result, err := leasesFromRows(rows)
	return result, err
}

// ClaimLease (lease.Store) claims the lease indicated by the input key,
// for the holder and duration indicated by the input request.
// The lease must not already be held, otherwise an error is returned.
func (s *Store) ClaimLease(ctx context.Context, key lease.Key, req lease.Request) error {
	if err := req.Validate(); err != nil {
		return errors.Trace(err)
	}

	q := `
INSERT INTO lease (uuid, lease_type_id, model_uuid, name, holder, start, expiry)
SELECT ?, id, ?, ?, ?, datetime('now'), datetime('now', ?) 
FROM   lease_type
WHERE  type = ?`[1:]

	stmt, err := s.getPrepared(ctx, "ClaimLease", q)
	if err != nil {
		return errors.Trace(err)
	}

	d := fmt.Sprintf("+%d seconds", int64(math.Ceil(req.Duration.Seconds())))
	uuid := utils.MustNewUUID().String()
	_, err = stmt.ExecContext(ctx, uuid, key.ModelUUID, key.Lease, req.Holder, d, key.Namespace)
	if database.IsErrConstraintUnique(err) {
		return lease.ErrHeld
	}
	return errors.Trace(err)
}

// ExtendLease (lease.Store) ensures the input lease will be held for at least
// the requested duration starting from now.
// If the input holder does not currently hold the lease, an error is returned.
func (s *Store) ExtendLease(ctx context.Context, key lease.Key, req lease.Request) error {
	if err := req.Validate(); err != nil {
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

	stmt, err := s.getPrepared(ctx, "ExtendLease", q)
	if err != nil {
		return errors.Trace(err)
	}

	d := fmt.Sprintf("+%d seconds", int64(math.Ceil(req.Duration.Seconds())))

	result, err := stmt.ExecContext(ctx, d, key.Namespace, key.ModelUUID, key.Lease, req.Holder)

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
}

// RevokeLease (lease.Store) deletes the lease from the store,
// provided it exists and is held by the input holder.
// If either of these conditions is false, an error is returned.
func (s *Store) RevokeLease(ctx context.Context, key lease.Key, holder string) error {
	q := `
DELETE FROM lease
WHERE  uuid = (
    SELECT l.uuid
    FROM   lease l JOIN lease_type t ON l.lease_type_id = t.id
    WHERE  t.type = ?
    AND    l.model_uuid = ?
    AND    l.name = ?
    AND    l.holder = ?
)`[1:]

	stmt, err := s.getPrepared(ctx, "RevokeLease", q)
	if err != nil {
		return errors.Trace(err)
	}

	result, err := stmt.ExecContext(ctx, key.Namespace, key.ModelUUID, key.Lease, holder)
	if err == nil {
		var affected int64
		affected, err = result.RowsAffected()
		if affected == 0 && err == nil {
			err = lease.ErrInvalid
		}
	}
	return errors.Trace(err)
}

// LeaseGroup (lease.Store) returns all leases
// for the input namespace and model.
func (s *Store) LeaseGroup(ctx context.Context, namespace, modelUUID string) (map[lease.Key]lease.Info, error) {
	q := `
SELECT t.type, l.model_uuid, l.name, l.holder, l.expiry
FROM   lease l JOIN lease_type t ON l.lease_type_id = t.id
WHERE  t.type = ?
AND    l.model_uuid = ?`[1:]

	stmt, err := s.getPrepared(ctx, "LeaseGroup", q)
	if err != nil {
		return nil, errors.Trace(err)
	}

	rows, err := stmt.QueryContext(ctx, namespace, modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	result, err := leasesFromRows(rows)
	return result, errors.Trace(err)
}

// PinLease (lease.Store) adds the input entity into the lease_pin table
// to indicate that the lease indicated by the input key must not expire,
// and that this entity requires such behaviour.
func (s *Store) PinLease(ctx context.Context, key lease.Key, entity string) error {
	q := `
INSERT INTO lease_pin (uuid, lease_uuid, entity_id)
SELECT ?, l.uuid, ?
FROM   lease l JOIN lease_type t ON l.lease_type_id = t.id
WHERE  t.type = ?
AND    l.model_uuid = ?
AND    l.name = ?`[1:]

	stmt, err := s.getPrepared(ctx, "PinLease", q)
	if err != nil {
		return errors.Trace(err)
	}

	_, err = stmt.ExecContext(ctx, utils.MustNewUUID().String(), entity, key.Namespace, key.ModelUUID, key.Lease)

	// If the lease is already pinned for this requester, it is a no-op.
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
func (s *Store) UnpinLease(ctx context.Context, key lease.Key, entity string) error {
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

	stmt, err := s.getPrepared(ctx, "UnpinLease", q)
	if err != nil {
		return errors.Trace(err)
	}

	_, err = stmt.ExecContext(ctx, key.Namespace, key.ModelUUID, key.Lease, entity)
	return errors.Trace(err)
}

// Pinned (lease.Store) returns all leases that are currently pinned,
// and the entities requiring such behaviour for them.
func (s *Store) Pinned(ctx context.Context) (map[lease.Key][]string, error) {
	q := `
SELECT   l.uuid, t.type, l.model_uuid, l.name, p.entity_id
FROM     lease l 
         JOIN lease_type t ON l.lease_type_id = t.id
         JOIN lease_pin p on l.uuid = p.lease_uuid
ORDER BY l.uuid`[1:]

	stmt, err := s.getPrepared(ctx, "Pinned", q)
	if err != nil {
		return nil, errors.Trace(err)
	}

	rows, err := stmt.QueryContext(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	seen := set.NewStrings()
	result := make(map[lease.Key][]string)
	for rows.Next() {
		var leaseUUID string
		var key lease.Key
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

// getPrepared returns a prepared statement for the input name,
// ensuring that the first call for a given name caches the statement.
// thereafter the statement is returned from the cache.
func (s *Store) getPrepared(ctx context.Context, name string, stmt string) (*sql.Stmt, error) {
	s.cacheMu.RLock()
	if cachedStmt, ok := s.cache[name]; ok {
		s.cacheMu.RUnlock()
		return cachedStmt, nil
	}
	s.cacheMu.RUnlock()

	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	prepared, err := s.db.PrepareContext(ctx, stmt)
	if err != nil {
		return nil, errors.Trace(err)
	}

	s.cache[name] = prepared
	return prepared, nil
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
