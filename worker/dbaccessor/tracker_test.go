// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbaccessor

import (
	"context"
	"database/sql"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/testing"
)

type trackedDBWorkerSuite struct {
	dbBaseSuite
}

var _ = gc.Suite(&trackedDBWorkerSuite{})

func (s *trackedDBWorkerSuite) TestWorkerStartup(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()
	s.expectClock()
	s.expectTimer(0)

	s.dbApp.EXPECT().Open(gomock.Any(), "controller").Return(s.DB(), nil)

	w, err := NewTrackedDBWorker(s.dbApp, "controller", WithClock(s.clock), WithLogger(s.logger))
	c.Assert(err, jc.ErrorIsNil)

	defer workertest.DirtyKill(c, w)

	workertest.CleanKill(c, w)
}

func (s *trackedDBWorkerSuite) TestWorkerDBIsNotNil(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()
	s.expectClock()
	s.expectTimer(0)

	s.dbApp.EXPECT().Open(gomock.Any(), "controller").Return(s.DB(), nil)

	w, err := s.newTrackedDBWorker(defaultPingDBFunc)
	c.Assert(err, jc.ErrorIsNil)

	defer workertest.DirtyKill(c, w)

	err = w.TxnNoRetry(context.Background(), func(_ context.Context, tx *sql.Tx) error {
		if tx == nil {
			return errors.New("nil transaction")
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	workertest.CleanKill(c, w)
}

func (s *trackedDBWorkerSuite) TestWorkerTxnIsNotNil(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()
	s.expectClock()
	s.expectTimer(0)

	s.dbApp.EXPECT().Open(gomock.Any(), "controller").Return(s.DB(), nil)

	w, err := s.newTrackedDBWorker(defaultPingDBFunc)
	c.Assert(err, jc.ErrorIsNil)

	defer workertest.DirtyKill(c, w)

	done := make(chan struct{})
	err = w.Txn(context.TODO(), func(ctx context.Context, tx *sql.Tx) error {
		defer close(done)

		c.Assert(tx, gc.NotNil)
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	select {
	case <-done:
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for DB callback")
	}

	workertest.CleanKill(c, w)
}

func (s *trackedDBWorkerSuite) TestWorkerAttemptsToVerifyDB(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()
	s.expectClock()
	done := s.expectTimer(1)

	s.timer.EXPECT().Reset(PollInterval).Times(1)
	s.dbApp.EXPECT().Open(gomock.Any(), "controller").Return(s.DB(), nil)

	var count uint64
	pingFn := func(context.Context, *sql.DB) error {
		atomic.AddUint64(&count, 1)
		return nil
	}

	w, err := s.newTrackedDBWorker(pingFn)
	c.Assert(err, jc.ErrorIsNil)

	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for DB callback")
	}

	// Attempt to use the new db, note there shouldn't be any leases in this db.
	tables := readTableNames(c, w)
	c.Assert(tables, SliceContains, "lease")

	workertest.CleanKill(c, w)

	c.Assert(count, gc.Equals, uint64(1))
	c.Assert(w.Err(), jc.ErrorIsNil)
}

func (s *trackedDBWorkerSuite) TestWorkerAttemptsToVerifyDBButSucceeds(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()
	s.expectClock()
	done := s.expectTimer(1)

	s.timer.EXPECT().Reset(PollInterval).Times(1)

	dbChange := make(chan struct{})
	s.dbApp.EXPECT().Open(gomock.Any(), "controller").Return(s.DB(), nil).Times(DefaultVerifyAttempts - 1)
	s.dbApp.EXPECT().Open(gomock.Any(), "controller").Return(s.DB(), nil).DoAndReturn(func(_ context.Context, _ string) (*sql.DB, error) {
		defer close(dbChange)
		return s.DB(), nil
	})

	var count uint64
	pingFn := func(context.Context, *sql.DB) error {
		val := atomic.AddUint64(&count, 1)

		if val == DefaultVerifyAttempts {
			return nil
		}
		return errors.New("boom")
	}

	w, err := s.newTrackedDBWorker(pingFn)
	c.Assert(err, jc.ErrorIsNil)

	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for DB callback")
	}

	// The db should have changed to the new db.
	select {
	case <-dbChange:
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for DB callback")
	}

	tables := readTableNames(c, w)
	c.Assert(tables, SliceContains, "lease")

	workertest.CleanKill(c, w)

	c.Assert(w.Err(), jc.ErrorIsNil)
}

func (s *trackedDBWorkerSuite) TestWorkerAttemptsToVerifyDBRepeatedly(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()
	s.expectClock()
	done := s.expectTimer(2)

	s.timer.EXPECT().Reset(PollInterval).Times(2)

	s.dbApp.EXPECT().Open(gomock.Any(), "controller").Return(s.DB(), nil)

	var count uint64
	pingFn := func(context.Context, *sql.DB) error {
		atomic.AddUint64(&count, 1)
		return nil
	}

	w, err := s.newTrackedDBWorker(pingFn)
	c.Assert(err, jc.ErrorIsNil)

	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for DB callback")
	}

	// Attempt to use the new db, note there shouldn't be any leases in this db.
	tables := readTableNames(c, w)
	c.Assert(tables, SliceContains, "lease")

	workertest.CleanKill(c, w)

	c.Assert(count, gc.Equals, uint64(2))
	c.Assert(w.Err(), jc.ErrorIsNil)
}

func (s *trackedDBWorkerSuite) TestWorkerAttemptsToVerifyDBButSucceedsWithDifferentDB(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()
	s.expectClock()
	done := s.expectTimer(1)

	s.timer.EXPECT().Reset(PollInterval).Times(1)

	dbChange := make(chan struct{})
	exp := s.dbApp.EXPECT()
	gomock.InOrder(
		exp.Open(gomock.Any(), "controller").Return(s.DB(), nil),
		exp.Open(gomock.Any(), "controller").Return(s.DB(), nil),
		exp.Open(gomock.Any(), "controller").DoAndReturn(func(_ context.Context, _ string) (*sql.DB, error) {
			defer close(dbChange)
			return s.NewCleanDB(c), nil
		}),
	)

	var count uint64
	pingFn := func(context.Context, *sql.DB) error {
		val := atomic.AddUint64(&count, 1)

		if val == DefaultVerifyAttempts {
			return nil
		}
		return errors.New("boom")
	}

	w, err := s.newTrackedDBWorker(pingFn)
	c.Assert(err, jc.ErrorIsNil)

	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for DB callback")
	}

	// Wait for the clean database to have been returned.
	select {
	case <-dbChange:
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for DB callback")
	}

	// There is a race potential race with the composition here, because
	// although the ping func may return a new database, it is not instantly
	// set as the worker's DB reference. We need to give it a chance.
	// In-theatre this will be OK, because a DB in an error state recoverable
	// by reconnecting will be replaced within the default retry strategy's
	// backoff/repeat loop.
	timeout := time.After(testing.ShortWait)
	tables := readTableNames(c, w)
loop:
	for {
		select {
		case <-timeout:
			c.Fatal("did not reach expected clean DB state")
		default:
			if set.NewStrings(tables...).Contains("lease") {
				tables = readTableNames(c, w)
			} else {
				break loop
			}
		}
	}

	workertest.CleanKill(c, w)
	c.Assert(w.Err(), jc.ErrorIsNil)
}

func (s *trackedDBWorkerSuite) TestWorkerAttemptsToVerifyDBButFails(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()
	s.expectClock()
	done := s.expectTimer(1)

	s.dbApp.EXPECT().Open(gomock.Any(), "controller").Return(s.DB(), nil).Times(DefaultVerifyAttempts)

	pingFn := func(context.Context, *sql.DB) error {
		return errors.New("boom")
	}

	w, err := s.newTrackedDBWorker(pingFn)
	c.Assert(err, jc.ErrorIsNil)

	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for DB callback")
	}

	c.Assert(w.Wait(), gc.ErrorMatches, "boom")
	c.Assert(w.Err(), gc.ErrorMatches, "boom")

	// Ensure that the DB is dead.
	err = w.Txn(context.TODO(), func(ctx context.Context, tx *sql.Tx) error {
		c.Fatal("failed if called")
		return nil
	})
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *trackedDBWorkerSuite) newTrackedDBWorker(pingFn func(context.Context, *sql.DB) error) (TrackedDB, error) {
	collector := NewMetricsCollector()
	return NewTrackedDBWorker(s.dbApp, "controller",
		WithClock(s.clock),
		WithLogger(s.logger),
		WithPingDBFunc(pingFn),
		WithMetricsCollector(collector),
	)
}

func readTableNames(c *gc.C, w coredatabase.TrackedDB) []string {
	// Attempt to use the new db, note there shouldn't be any leases in this
	// db.
	var tables []string
	err := w.Txn(context.TODO(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.Query("SELECT tbl_name FROM sqlite_schema")
		c.Assert(err, jc.ErrorIsNil)
		defer rows.Close()

		for rows.Next() {
			var table string
			err = rows.Scan(&table)
			c.Assert(err, jc.ErrorIsNil)
			tables = append(tables, table)
		}

		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	return set.NewStrings(tables...).SortedValues()
}

type sliceContainsChecker[T comparable] struct {
	*gc.CheckerInfo
}

var SliceContains gc.Checker = &sliceContainsChecker[string]{
	&gc.CheckerInfo{Name: "SliceContains", Params: []string{"obtained", "expected"}},
}

func (checker *sliceContainsChecker[T]) Check(params []interface{}, names []string) (result bool, error string) {
	expected, ok := params[1].(T)
	if !ok {
		var t T
		return false, fmt.Sprintf("expected must be %T", t)
	}

	obtained, ok := params[0].([]T)
	if !ok {
		var t T
		return false, fmt.Sprintf("Obtained value is not a []%T", t)
	}

	for _, o := range obtained {
		if o == expected {
			return true, ""
		}
	}
	return false, ""
}
