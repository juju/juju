// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/macaroon"
	macaroonerrors "github.com/juju/juju/domain/macaroon/errors"
	"github.com/juju/juju/internal/database"
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
func (st *RootKeyState) GetKey(ctx context.Context, id []byte) (macaroon.RootKey, error) {
	db, err := st.DB()
	if err != nil {
		return macaroon.RootKey{}, errors.Trace(err)
	}
	key := rootKeyID{ID: id}
	getKeyStmt, err := st.Prepare("SELECT &rootKey.* FROM macaroon_root_key WHERE id = $rootKeyID.id", rootKey{}, key)
	if err != nil {
		return macaroon.RootKey{}, errors.Annotate(err, "preparing get root key statement")
	}

	var result rootKey
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, getKeyStmt, key).Get(&result)
		if database.IsErrNotFound(err) {
			return errors.Annotatef(macaroonerrors.KeyNotFound, "key with id %s", string(id))
		}
		return err
	})
	return macaroon.RootKey{
		ID:      result.ID,
		Created: result.Created,
		Expires: result.Expires,
		RootKey: result.RootKey,
	}, errors.Trace(err)
}

// FindLatestKey returns the most recently created root key k following all
// the conditions:
//
// k.Created >= createdAfter
// k.Expires >= expiresAfter
// k.Expires <= expiresBefore
//
// If no such key was found, return a macaroonerrors.KeyNotFound error
func (st *RootKeyState) FindLatestKey(ctx context.Context, createdAfter, expiresAfter, expiresBefore time.Time) (macaroon.RootKey, error) {
	db, err := st.DB()
	if err != nil {
		return macaroon.RootKey{}, errors.Trace(err)
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
		return macaroon.RootKey{}, errors.Annotatef(err, "preparing get latest key statement")
	}

	var result rootKey
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, m).Get(&result)
		if database.IsErrNotFound(err) {
			return macaroonerrors.KeyNotFound
		}
		return err
	})
	if err != nil {
		return macaroon.RootKey{}, errors.Trace(err)
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
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	row := rootKey{
		ID:      key.ID,
		Created: key.Created,
		Expires: key.Expires,
		RootKey: key.RootKey,
	}
	insertKeyStmt, err := st.Prepare("INSERT INTO macaroon_root_key (*) VALUES ($rootKey.*)", row)
	if err != nil {
		return errors.Annotate(err, "preparing insert root key statement")
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, insertKeyStmt, row).Run()
		if database.IsErrConstraintPrimaryKey(err) {
			return errors.Annotatef(macaroonerrors.KeyAlreadyExists, "key with id %s", string(key.ID))
		}
		return err
	})
	return errors.Trace(err)
}

// RemoveKeysExpiredBefore removes all root keys from state with an expiry
// before the provided cutoff time
func (st *RootKeyState) RemoveKeysExpiredBefore(ctx context.Context, cutoff time.Time) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	m := sqlair.M{
		"cutoff": cutoff,
	}

	removeExpiredStmt, err := st.Prepare("DELETE FROM macaroon_root_key WHERE expires_at < $M.cutoff", m)
	if err != nil {
		return errors.Annotatef(err, "preparing remove expired root key statement")
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, removeExpiredStmt, m).Run()
	})

	return errors.Trace(err)
}
