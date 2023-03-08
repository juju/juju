// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package txn_test

import (
	"context"
	"database/sql"

	jc "github.com/juju/testing/checkers"
	"github.com/mattn/go-sqlite3"
	gc "gopkg.in/check.v1"

	"github.com/juju/errors"
	"github.com/juju/juju/database/testing"
	"github.com/juju/juju/database/txn"
)

type transactionerSuite struct {
	testing.ControllerSuite
}

var _ = gc.Suite(&transactionerSuite{})

func (s *transactionerSuite) TestTxn(c *gc.C) {
	transactioner := txn.NewTransactioner()

	err := transactioner.Txn(context.TODO(), s.DB(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, "SELECT 1")
		if err != nil {
			return errors.Trace(err)
		}
		defer rows.Close()
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *transactionerSuite) TestTxnWithCancelledContext(c *gc.C) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	transactioner := txn.NewTransactioner()

	err := transactioner.Txn(ctx, s.DB(), func(ctx context.Context, tx *sql.Tx) error {
		c.Fatal("should not be called")
		return nil
	})
	c.Assert(err, gc.ErrorMatches, "context canceled")
}

func (s *transactionerSuite) TestTxnInserts(c *gc.C) {
	transactioner := txn.NewTransactioner()

	s.createTable(c)

	err := transactioner.Txn(context.TODO(), s.DB(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "INSERT INTO foo (id, name) VALUES (1, 'test')")
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	// Now verify that the transaction was rolled back.
	rows, err := s.DB().Query("SELECT COUNT(*) FROM foo")
	c.Assert(err, jc.ErrorIsNil)

	defer rows.Close()

	for !rows.Next() {
		var n int
		err := rows.Scan(&n)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(n, gc.Equals, 1)
	}
}

func (s *transactionerSuite) TestTxnRollback(c *gc.C) {
	transactioner := txn.NewTransactioner()

	s.createTable(c)

	err := transactioner.Txn(context.TODO(), s.DB(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "INSERT INTO foo (id, name) VALUES (1, 'test')")
		if err != nil {
			return errors.Trace(err)
		}
		return errors.Errorf("fail")
	})
	c.Assert(err, gc.ErrorMatches, "fail")

	// Now verify that the transaction was rolled back.
	rows, err := s.DB().Query("SELECT COUNT(*) FROM foo")
	c.Assert(err, jc.ErrorIsNil)

	defer rows.Close()

	for !rows.Next() {
		var n int
		err := rows.Scan(&n)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(n, gc.Equals, 0)
	}
}

func (s *transactionerSuite) TestRetryForNonRetryableError(c *gc.C) {
	transactioner := txn.NewTransactioner()

	var count int
	err := transactioner.Retry(context.TODO(), func() error {
		count++
		return errors.Errorf("fail")
	})
	c.Assert(err, gc.ErrorMatches, "fail")
	c.Assert(count, gc.Equals, 1)
}

func (s *transactionerSuite) TestRetryWithACancelledContext(c *gc.C) {
	ctx, cancel := context.WithCancel(context.Background())

	transactioner := txn.NewTransactioner()

	var count int
	err := transactioner.Retry(ctx, func() error {
		defer cancel()

		count++
		return errors.Errorf("fail")
	})
	c.Assert(err, gc.ErrorMatches, "fail")
	c.Assert(count, gc.Equals, 1)
}

func (s *transactionerSuite) TestRetryForRetryableError(c *gc.C) {
	transactioner := txn.NewTransactioner()

	var count int
	err := transactioner.Retry(context.TODO(), func() error {
		count++
		return sqlite3.ErrBusy
	})
	c.Assert(err, gc.ErrorMatches, "attempt count exceeded: .*")
	c.Assert(count, gc.Equals, 250)
}

func (s *transactionerSuite) createTable(c *gc.C) {
	_, err := s.DB().Exec("CREATE TEMP TABLE foo (id INT PRIMARY KEY, name VARCHAR(255))")
	c.Assert(err, jc.ErrorIsNil)
}
