// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"
	"gopkg.in/tomb.v2"

	coredatabase "github.com/juju/juju/core/database"
)

type TestTrackedDB struct {
	tomb    tomb.Tomb
	factory coredatabase.TxnRunnerFactory
}

// NewTestTrackedDB returns a tracked db worker which returns a tracked db
// attached to the test suite.
func NewTestTrackedDB(f coredatabase.TxnRunnerFactory) *TestTrackedDB {
	w := &TestTrackedDB{
		factory: f,
	}
	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		return tomb.ErrDying
	})
	return w
}

func (t *TestTrackedDB) Kill() {
	t.tomb.Kill(nil)
}

func (t *TestTrackedDB) Wait() error {
	return t.tomb.Wait()
}

func (t *TestTrackedDB) Txn(ctx context.Context, fn func(context.Context, *sqlair.TX) error) error {
	db, err := t.factory(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	return db.Txn(ctx, fn)
}

func (t *TestTrackedDB) StdTxn(ctx context.Context, fn func(context.Context, *sql.Tx) error) error {
	db, err := t.factory(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	return db.StdTxn(ctx, fn)
}
