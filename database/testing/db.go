// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"
	"database/sql"

	"github.com/juju/errors"
	"github.com/juju/juju/database/txn"
)

// trackedDB is used for testing purposes.
type trackedDB struct {
	db *sql.DB
}

func (t *trackedDB) DB(fn func(*sql.DB) error) error {
	return fn(t.db)
}

func (t *trackedDB) Txn(ctx context.Context, fn func(context.Context, *sql.Tx) error) error {
	return t.DB(func(db *sql.DB) error {
		// TODO (stickupkid): Implement retries for tests?
		return defaultTransactioner.Txn(ctx, db, fn)
	})
}

func (t *trackedDB) PrepareStmts(fn func(*sql.DB) error) (func(), error) {
	err := t.DB(func(db *sql.DB) error {
		return fn(db)
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	// TODO (stickupkid): maybe do something here?
	return func() {}, nil
}

func (t *trackedDB) Err() error {
	return nil
}

var (
	defaultTransactioner = txn.NewTransactioner()
)
