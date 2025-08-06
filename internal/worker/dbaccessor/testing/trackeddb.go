// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"
	_ "github.com/mattn/go-sqlite3"
	"gopkg.in/tomb.v2"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/internal/worker/dbaccessor"
)

// NewTrackedDB returns a tracked db worker which returns a tracked db
// attached to the test suite.
func NewTrackedDB(f func() (coredatabase.TxnRunner, error)) dbaccessor.TrackedDB {
	w := &testTrackedDB{
		txnRunnerFactory: f,
	}
	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		return tomb.ErrDying
	})
	return w
}

type testTrackedDB struct {
	tomb             tomb.Tomb
	txnRunnerFactory func() (coredatabase.TxnRunner, error)
}

func (t *testTrackedDB) Kill() {
	t.tomb.Kill(nil)
}

func (t *testTrackedDB) Wait() error {
	return t.tomb.Wait()
}

func (t *testTrackedDB) Txn(ctx context.Context, fn func(context.Context, *sqlair.TX) error) error {
	db, err := t.txnRunnerFactory()
	if err != nil {
		return errors.Trace(err)
	}
	return db.Txn(ctx, fn)
}

func (t *testTrackedDB) StdTxn(ctx context.Context, fn func(context.Context, *sql.Tx) error) error {
	db, err := t.txnRunnerFactory()
	if err != nil {
		return errors.Trace(err)
	}
	return db.StdTxn(ctx, fn)
}

func (t *testTrackedDB) Dying() <-chan struct{} {
	return t.tomb.Dying()
}
