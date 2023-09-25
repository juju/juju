// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package txn_test

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"github.com/juju/errors"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/mattn/go-sqlite3"
	"golang.org/x/sync/semaphore"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/database/testing"
	"github.com/juju/juju/internal/database/txn"
)

type transactionRunnerSuite struct {
	testing.DqliteSuite
}

var _ = gc.Suite(&transactionRunnerSuite{})

func (s *transactionRunnerSuite) TestTxn(c *gc.C) {
	runner := txn.NewRetryingTxnRunner()

	err := runner.StdTxn(context.TODO(), s.DB(), func(ctx context.Context, tx *sql.Tx) error {
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

	runner := txn.NewRetryingTxnRunner()

	err := runner.StdTxn(ctx, s.DB(), func(ctx context.Context, tx *sql.Tx) error {
		c.Fatal("should not be called")
		return nil
	})
	c.Assert(err, gc.ErrorMatches, "context canceled")
}

func (s *transactionRunnerSuite) TestTxnParallelCancelledContext(c *gc.C) {
	runner := txn.NewRetryingTxnRunner(txn.WithSemaphore(semaphore.NewWeighted(1)))

	var wg sync.WaitGroup
	wg.Add(2)

	// The following two goroutines will attempt to start a transaction
	// concurrently. The first one will start, and the second one will be
	// blocked until the first one has completed. We can then ensure that
	// the second one is cancelled, because the context is cancelled.
	sync := make(chan struct{})
	step := make(chan struct{})
	go func() {
		defer wg.Done()

		err := runner.StdTxn(context.Background(), s.DB(), func(ctx context.Context, tx *sql.Tx) error {
			close(sync)

			select {
			case <-time.After(jujutesting.ShortWait):
			case <-step:
			}
			return nil
		})
		c.Assert(err, jc.ErrorIsNil)
	}()

	go func() {
		defer wg.Done()

		// Wait until the first transaction has started, before attempting a
		// second one.
		select {
		case <-sync:
		case <-time.After(jujutesting.ShortWait):
			c.Fatal("should not be called")
		}

		ctx, cancel := context.WithCancel(context.Background())

		// Force the cancel to happen after the transaction has started.
		cancel()
		err := runner.StdTxn(ctx, s.DB(), func(ctx context.Context, tx *sql.Tx) error {
			c.Fatal("should not be called")
			return nil
		})
		c.Assert(err, gc.ErrorMatches, "context canceled")

		close(step)
	}()

	// The following ensures that we don't block whilst waiting for the tests
	// to complete.
	wait := make(chan struct{})
	go func() {
		wg.Wait()
		close(wait)
	}()
	select {
	case <-wait:
	case <-time.After(jujutesting.LongWait):
		c.Fatal("failed waiting to complete")
	}
}

func (s *transactionRunnerSuite) TestTxnInserts(c *gc.C) {
	runner := txn.NewRetryingTxnRunner()

	s.createTable(c)

	err := runner.StdTxn(context.TODO(), s.DB(), func(ctx context.Context, tx *sql.Tx) error {
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
	runner := txn.NewRetryingTxnRunner()

	s.createTable(c)

	err := runner.StdTxn(context.TODO(), s.DB(), func(ctx context.Context, tx *sql.Tx) error {
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
	runner := txn.NewRetryingTxnRunner()

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

	runner := txn.NewRetryingTxnRunner()

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
	runner := txn.NewRetryingTxnRunner()

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
