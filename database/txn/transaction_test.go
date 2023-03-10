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

type transactionRunnerSuite struct {
	testing.ControllerSuite
}

var _ = gc.Suite(&transactionRunnerSuite{})

func (s *transactionRunnerSuite) TestTxn(c *gc.C) {
	runner := txn.NewTransactionRunner()

	err := runner.Txn(context.TODO(), s.DB(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, "SELECT 1")
		if err != nil {
			return errors.Trace(err)
		}
		defer rows.Close()
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *transactionRunnerSuite) TestTxnWithCancelledContext(c *gc.C) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	runner := txn.NewTransactionRunner()

	err := runner.Txn(ctx, s.DB(), func(ctx context.Context, tx *sql.Tx) error {
		c.Fatal("should not be called")
		return nil
	})
	c.Assert(err, gc.ErrorMatches, "context canceled")
}

func (s *transactionRunnerSuite) TestTxnInserts(c *gc.C) {
	runner := txn.NewTransactionRunner()

	s.createTable(c)

	err := runner.Txn(context.TODO(), s.DB(), func(ctx context.Context, tx *sql.Tx) error {
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

func (s *transactionRunnerSuite) TestTxnRollback(c *gc.C) {
	runner := txn.NewTransactionRunner()

	s.createTable(c)

	err := runner.Txn(context.TODO(), s.DB(), func(ctx context.Context, tx *sql.Tx) error {
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

func (s *transactionRunnerSuite) TestRetryForNonRetryableError(c *gc.C) {
	runner := txn.NewTransactionRunner()

	var count int
	err := runner.Retry(context.TODO(), func() error {
		count++
		return errors.Errorf("fail")
	})
	c.Assert(err, gc.ErrorMatches, "fail")
	c.Assert(count, gc.Equals, 1)
}

func (s *transactionRunnerSuite) TestRetryWithACancelledContext(c *gc.C) {
	ctx, cancel := context.WithCancel(context.Background())

	runner := txn.NewTransactionRunner()

	var count int
	err := runner.Retry(ctx, func() error {
		defer cancel()

		count++
		return errors.Errorf("fail")
	})
	c.Assert(err, gc.ErrorMatches, "fail")
	c.Assert(count, gc.Equals, 1)
}

func (s *transactionRunnerSuite) TestRetryForRetryableError(c *gc.C) {
	runner := txn.NewTransactionRunner()

	var count int
	err := runner.Retry(context.TODO(), func() error {
		count++
		return sqlite3.ErrBusy
	})
	c.Assert(err, gc.ErrorMatches, "attempt count exceeded: .*")
	c.Assert(count, gc.Equals, 250)
}

func (s *transactionRunnerSuite) createTable(c *gc.C) {
	_, err := s.DB().Exec("CREATE TEMP TABLE foo (id INT PRIMARY KEY, name VARCHAR(255))")
	c.Assert(err, jc.ErrorIsNil)
}
