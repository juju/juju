// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package txn_test

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/mattn/go-sqlite3"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/database/testing"
	"github.com/juju/juju/internal/database/txn"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type transactionRunnerSuite struct {
	testing.DqliteSuite

	clock *MockClock
}

var _ = tc.Suite(&transactionRunnerSuite{})

func (s *transactionRunnerSuite) TestTxn(c *tc.C) {
	runner := txn.NewRetryingTxnRunner()

	err := runner.StdTxn(c.Context(), s.DB(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, "SELECT 1")
		if err != nil {
			return errors.Trace(err)
		}
		defer rows.Close()
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
}

type logRecorder struct {
	logger.Logger

	builder *strings.Builder

	c *tc.C
}

func (l logRecorder) IsLevelEnabled(level logger.Level) bool {
	return true
}

func (l logRecorder) Tracef(ctx context.Context, format string, args ...interface{}) {
	l.c.Logf(format, args...)
	l.builder.WriteString(fmt.Sprintf(format, args...))
	l.builder.WriteString("\n")
}

func (s *transactionRunnerSuite) TestTxnLogging(c *tc.C) {
	if _, isSQLite := s.DB().Driver().(*sqlite3.SQLiteDriver); isSQLite {
		c.Skip("TODO: log tracer is broken on sqlite")
	}

	buffer := new(strings.Builder)
	runner := txn.NewRetryingTxnRunner(txn.WithLogger(logRecorder{
		builder: buffer,
		c:       c,
	}))

	err := runner.StdTxn(c.Context(), s.DB(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, "SELECT 1")
		if err != nil {
			return errors.Trace(err)
		}
		defer rows.Close()
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(buffer.String(), tc.Equals, `
running txn (id: 1) with query: BEGIN
running txn (id: 1) with query: SELECT 1
running txn (id: 1) with query: COMMIT
`[1:])
}

func (s *transactionRunnerSuite) TestTxnWithCancelledContext(c *tc.C) {
	ctx, cancel := context.WithCancel(c.Context())
	cancel()

	runner := txn.NewRetryingTxnRunner()

	err := runner.StdTxn(ctx, s.DB(), func(ctx context.Context, tx *sql.Tx) error {
		c.Fatal("should not be called")
		return nil
	})
	c.Assert(err, tc.ErrorMatches, "context canceled")
}

func (s *transactionRunnerSuite) TestTxnParallelCancelledContext(c *tc.C) {
	runner := txn.NewRetryingTxnRunner()

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

		err := runner.StdTxn(c.Context(), s.DB(), func(ctx context.Context, tx *sql.Tx) error {
			close(sync)

			select {
			case <-time.After(testhelpers.ShortWait):
			case <-step:
			}
			return nil
		})
		c.Assert(err, tc.ErrorIsNil)
	}()

	go func() {
		defer wg.Done()

		// Wait until the first transaction has started, before attempting a
		// second one.
		select {
		case <-sync:
		case <-time.After(testhelpers.ShortWait):
			c.Fatal("should not be called")
		}

		ctx, cancel := context.WithCancel(c.Context())

		// Force the cancel to happen after the transaction has started.
		cancel()
		err := runner.StdTxn(ctx, s.DB(), func(ctx context.Context, tx *sql.Tx) error {
			c.Fatal("should not be called")
			return nil
		})
		c.Assert(err, tc.ErrorMatches, "context canceled")

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
	case <-time.After(testhelpers.LongWait):
		c.Fatal("failed waiting to complete")
	}
}

func (s *transactionRunnerSuite) TestTxnInserts(c *tc.C) {
	runner := txn.NewRetryingTxnRunner()

	s.createTable(c)

	err := runner.StdTxn(c.Context(), s.DB(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "INSERT INTO foo (id, name) VALUES (1, 'test')")
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	// Now verify that the transaction was rolled back.
	rows, err := s.DB().Query("SELECT COUNT(*) FROM foo")
	c.Assert(err, tc.ErrorIsNil)

	defer rows.Close()

	for !rows.Next() {
		var n int
		err := rows.Scan(&n)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(n, tc.Equals, 1)
	}
}

func (s *transactionRunnerSuite) TestTxnRollback(c *tc.C) {
	runner := txn.NewRetryingTxnRunner()

	s.createTable(c)

	err := runner.StdTxn(c.Context(), s.DB(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "INSERT INTO foo (id, name) VALUES (1, 'test')")
		if err != nil {
			return errors.Trace(err)
		}
		return errors.Errorf("fail")
	})
	c.Assert(err, tc.ErrorMatches, "fail")

	// Now verify that the transaction was rolled back.
	rows, err := s.DB().Query("SELECT COUNT(*) FROM foo")
	c.Assert(err, tc.ErrorIsNil)

	defer rows.Close()

	for !rows.Next() {
		var n int
		err := rows.Scan(&n)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(n, tc.Equals, 0)
	}
}

func (s *transactionRunnerSuite) TestRetryForNonRetryableError(c *tc.C) {
	runner := txn.NewRetryingTxnRunner()

	var count int
	err := runner.Retry(c.Context(), func() error {
		count++
		return errors.Errorf("fail")
	})
	c.Assert(err, tc.ErrorMatches, "fail")
	c.Assert(count, tc.Equals, 1)
}

func (s *transactionRunnerSuite) TestRetryWithACancelledContext(c *tc.C) {
	ctx, cancel := context.WithCancel(c.Context())

	runner := txn.NewRetryingTxnRunner()

	var count int
	err := runner.Retry(ctx, func() error {
		defer cancel()

		count++
		return errors.Errorf("fail")
	})
	c.Assert(err, tc.ErrorMatches, "fail")
	c.Assert(count, tc.Equals, 1)
}

func (s *transactionRunnerSuite) TestRetryForRetryableError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.clock.EXPECT().Now().Return(time.Now()).AnyTimes()
	s.clock.EXPECT().After(gomock.Any()).DoAndReturn(func(d time.Duration) <-chan time.Time {
		ch := make(chan time.Time)
		close(ch)
		return ch
	}).AnyTimes()

	runner := txn.NewRetryingTxnRunner(txn.WithRetryStrategy(txn.DefaultRetryStrategy(s.clock, loggertesting.WrapCheckLog(c))))

	var count int
	err := runner.Retry(c.Context(), func() error {
		count++
		return sqlite3.ErrBusy
	})
	c.Assert(err, tc.ErrorMatches, "attempt count exceeded: .*")
	c.Assert(count, tc.Equals, 250)
}

func (s *transactionRunnerSuite) createTable(c *tc.C) {
	_, err := s.DB().Exec("CREATE TEMP TABLE foo (id INT PRIMARY KEY, name VARCHAR(255))")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *transactionRunnerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.clock = NewMockClock(ctrl)

	return ctrl
}
