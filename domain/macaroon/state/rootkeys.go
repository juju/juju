// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"time"

	"github.com/canonical/sqlair"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/macaroon"
	macaroonerrors "github.com/juju/juju/domain/macaroon/errors"
	"github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
)

// RootKeyState describes the persistence layer for
// macaroon root keys
type RootKeyState struct {
	*domain.StateBase
}

// NewRootKeyState return a new macaroon root key state reference
func NewRootKeyState(factory coredatabase.TxnRunnerFactory) *RootKeyState {
	return &RootKeyState{
		StateBase: domain.NewStateBase(factory),
	}
}

// GetKey gets the key with a given id from dqlite. If not key is found, a
// macaroonerrors.KeyNotFound error is returned.
func (st *RootKeyState) GetKey(ctx context.Context, id []byte, now time.Time) (macaroon.RootKey, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return macaroon.RootKey{}, errors.Capture(err)
	}
	key := rootKeyID{ID: id}
	getKeyStmt, err := st.Prepare("SELECT &rootKey.* FROM macaroon_root_key WHERE id = $rootKeyID.id", rootKey{}, key)
	if err != nil {
		return macaroon.RootKey{}, errors.Errorf("preparing get root key statement: %w", err)
	}

	var result rootKey
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := st.removeKeysExpiredBefore(ctx, tx, now)
		if err != nil {
			return domain.CoerceError(err)
		}
		err = tx.Query(ctx, getKeyStmt, key).Get(&result)
		if database.IsErrNotFound(err) {
			return errors.Errorf("key with id %s: %w", string(id), macaroonerrors.KeyNotFound)
		}
		return err
	})
	return macaroon.RootKey{
		ID:      result.ID,
		Created: result.Created,
		Expires: result.Expires,
		RootKey: result.RootKey,
	}, errors.Capture(err)
}

// FindLatestKey returns the most recently created root key k following all
// the conditions:
//
// k.Created >= createdAfter
// k.Expires >= expiresAfter
// k.Expires <= expiresBefore
//
// If no such key was found, return a macaroonerrors.KeyNotFound error
func (st *RootKeyState) FindLatestKey(ctx context.Context, createdAfter, expiresAfter, expiresBefore, now time.Time) (macaroon.RootKey, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return macaroon.RootKey{}, errors.Capture(err)
	}

	m := sqlair.M{
		"created_after":  createdAfter,
		"expires_after":  expiresAfter,
		"expires_before": expiresBefore,
	}

	q := `
SELECT &rootKey.*
FROM   macaroon_root_key
WHERE  created_at >= $M.created_after
AND    expires_at >= $M.expires_after
AND    expires_at <= $M.expires_before
ORDER BY created_at DESC
`
	stmt, err := st.Prepare(q, rootKey{}, m)
	if err != nil {
		return macaroon.RootKey{}, errors.Errorf("preparing get latest key statement: %w", err)
	}

	var result rootKey
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := st.removeKeysExpiredBefore(ctx, tx, now)
		if err != nil {
			return domain.CoerceError(err)
		}
		err = tx.Query(ctx, stmt, m).Get(&result)
		if database.IsErrNotFound(err) {
			return macaroonerrors.KeyNotFound
		}
		return err
	})
	if err != nil {
		return macaroon.RootKey{}, errors.Capture(err)
	}
	return macaroon.RootKey{
		ID:      result.ID,
		Created: result.Created,
		Expires: result.Expires,
		RootKey: result.RootKey,
	}, nil
}

// InsertKey inserts the given root key into dqlite. If a key with matching
// id already exists, return a macaroonerrors.KeyAlreadyExists error.
func (st *RootKeyState) InsertKey(ctx context.Context, key macaroon.RootKey) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	row := rootKey{
		ID:      key.ID,
		Created: key.Created,
		Expires: key.Expires,
		RootKey: key.RootKey,
	}
	insertKeyStmt, err := st.Prepare("INSERT INTO macaroon_root_key (*) VALUES ($rootKey.*)", row)
	if err != nil {
		return errors.Errorf("preparing insert root key statement: %w", err)
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, insertKeyStmt, row).Run()
		if database.IsErrConstraintPrimaryKey(err) {
			return errors.Errorf("key with id %s: %w", string(key.ID), macaroonerrors.KeyAlreadyExists)
		}
		return err
	})
	return errors.Capture(err)
}

// removeKeysExpiredBefore removes all root keys from state with an expiry
// before the provided cutoff time
func (st *RootKeyState) removeKeysExpiredBefore(ctx context.Context, tx *sqlair.TX, cutoff time.Time) error {
	m := sqlair.M{
		"cutoff": cutoff,
	}

	removeExpiredStmt, err := st.Prepare("DELETE FROM macaroon_root_key WHERE expires_at < $M.cutoff", m)
	if err != nil {
		return errors.Errorf("preparing remove expired root key statement: %w", err)
	}
	return tx.Query(ctx, removeExpiredStmt, m).Run()
}
