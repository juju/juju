// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import (
	"context"
	"database/sql"
	"fmt"
	"math"

	"github.com/juju/utils/v3"

	"github.com/juju/errors"
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

// LeaseGroup returns all leases for the input namespace and model.
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

func (s *DBStore) PinLease(lease Key, entity string, stop <-chan struct{}) error {
	panic("implement me")
}

func (s *DBStore) UnpinLease(lease Key, entity string, stop <-chan struct{}) error {
	panic("implement me")
}

func (s *DBStore) Pinned() (map[Key][]string, error) {
	panic("implement me")
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
