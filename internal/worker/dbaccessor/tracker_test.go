// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbaccessor

import (
	"context"
	"database/sql"
	"fmt"
	"sync/atomic"
	stdtesting "testing"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/clock/testclock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/internal/testing"
)

// Ensure that the trackedDBWorker is a killableWorker.
var _ killableWorker = (*trackedDBWorker)(nil)

type trackedDBWorkerSuite struct {
	dbBaseSuite

	states chan string
}

func TestTrackedDBWorkerSuite(t *stdtesting.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &trackedDBWorkerSuite{})
}

func (s *trackedDBWorkerSuite) TestWorkerStartup(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()
	defer s.expectTimer(0)()

	s.dbApp.EXPECT().Open(gomock.Any(), "controller").Return(s.DB(), nil)

	w, err := NewTrackedDBWorker(c.Context(), s.dbApp, "controller", WithClock(s.clock), WithLogger(s.logger))
	c.Assert(err, tc.ErrorIsNil)

	workertest.CleanKill(c, w)
}

func (s *trackedDBWorkerSuite) TestWorkerReport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()
	defer s.expectTimer(0)()

	s.dbApp.EXPECT().Open(gomock.Any(), "controller").Return(s.DB(), nil)

	w, err := NewTrackedDBWorker(c.Context(), s.dbApp, "controller", WithClock(s.clock), WithLogger(s.logger))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	report := w.(interface{ Report() map[string]any }).Report()
	c.Assert(report, MapHasKeys, []string{
		"db-replacements",
		"max-ping-duration",
		"last-ping-attempts",
		"last-ping-duration",
	})

	workertest.CleanKill(c, w)
}

func (s *trackedDBWorkerSuite) TestWorkerDBIsNotNil(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()
	defer s.expectTimer(0)()

	s.dbApp.EXPECT().Open(gomock.Any(), "controller").Return(s.DB(), nil)

	w, err := s.newTrackedDBWorker(c, defaultPingDBFunc)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	err = w.StdTxn(c.Context(), func(_ context.Context, tx *sql.Tx) error {
		if tx == nil {
			return errors.New("nil transaction")
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	workertest.CleanKill(c, w)
}

func (s *trackedDBWorkerSuite) TestWorkerTxnIsNotNil(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()
	defer s.expectTimer(0)()

	s.dbApp.EXPECT().Open(gomock.Any(), "controller").Return(s.DB(), nil)

	w, err := s.newTrackedDBWorker(c, defaultPingDBFunc)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	done := make(chan struct{})
	err = w.Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		defer close(done)

		if tx == nil {
			return errors.New("nil transaction")
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	select {
	case <-done:
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for DB callback")
	}

	workertest.CleanKill(c, w)
}

func (s *trackedDBWorkerSuite) TestWorkerStdTxnIsNotNil(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()
	defer s.expectTimer(0)()

	s.dbApp.EXPECT().Open(gomock.Any(), "controller").Return(s.DB(), nil)

	w, err := s.newTrackedDBWorker(c, defaultPingDBFunc)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	done := make(chan struct{})
	err = w.StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		defer close(done)

		if tx == nil {
			return errors.New("nil transaction")
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	select {
	case <-done:
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for DB callback")
	}

	workertest.CleanKill(c, w)
}

func (s *trackedDBWorkerSuite) TestWorkerAttemptsToVerifyDB(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// This test uses a dialated wall clock to test retries.
	s.clock = testclock.NewDilatedWallClock(time.Millisecond)

	s.dbApp.EXPECT().Open(gomock.Any(), "controller").Return(s.DB(), nil)

	done := make(chan struct{})
	var count uint64
	pingFn := func(context.Context, *sql.DB) error {
		if atomic.AddUint64(&count, 1) == 1 {
			close(done)
		}
		return nil
	}

	w, err := s.newTrackedDBWorker(c, pingFn)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	// Attempt to use the new db, note there shouldn't be any leases in this db.
	tables := readTableNames(c, w)
	c.Assert(tables, SliceContains, "lease")

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatal("timed out waiting for multiple db verify")
	}

	workertest.CleanKill(c, w)

	c.Assert(count, tc.GreaterThan, uint64(0))
}

func (s *trackedDBWorkerSuite) TestWorkerAttemptsToVerifyDBButSucceeds(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()
	defer s.expectTimer(1)()

	s.timer.EXPECT().Reset(gomock.Any()).Times(1)

	dbReady := make(chan struct{})
	s.dbApp.EXPECT().Open(gomock.Any(), "controller").Return(s.DB(), nil).Times(DefaultVerifyAttempts)

	var count uint64
	pingFn := func(context.Context, *sql.DB) error {
		val := atomic.AddUint64(&count, 1)

		if val == DefaultVerifyAttempts {
			defer close(dbReady)
			return nil
		}
		return errors.New("boom")
	}

	w, err := s.newTrackedDBWorker(c, pingFn)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	// The db should wait to a successful ping after several attempts
	select {
	case <-dbReady:
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for DB callback")
	}

	tables := readTableNames(c, w)
	c.Assert(tables, SliceContains, "lease")

	workertest.CleanKill(c, w)
}

func (s *trackedDBWorkerSuite) TestWorkerAttemptsToVerifyDBRepeatedly(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// This test uses a dialated wall clock to test retries.
	s.clock = testclock.NewDilatedWallClock(time.Millisecond)

	s.dbApp.EXPECT().Open(gomock.Any(), "controller").Return(s.DB(), nil)

	done := make(chan struct{})
	var count uint64
	pingFn := func(context.Context, *sql.DB) error {
		if atomic.AddUint64(&count, 1) == 2 {
			close(done)
		}
		return nil
	}

	w, err := s.newTrackedDBWorker(c, pingFn)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	// Attempt to use the new db, note there shouldn't be any leases in this db.
	tables := readTableNames(c, w)
	c.Assert(tables, SliceContains, "lease")

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatal("timed out waiting for multiple db verify")
	}

	workertest.CleanKill(c, w)

	c.Assert(count, tc.GreaterThan, uint64(1))
}

func (s *trackedDBWorkerSuite) TestWorkerAttemptsToVerifyDBButSucceedsWithDifferentDB(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()
	defer s.expectTimer(1)()

	s.timer.EXPECT().Reset(gomock.Any()).Times(1)

	exp := s.dbApp.EXPECT()
	gomock.InOrder(
		exp.Open(gomock.Any(), "controller").Return(s.DB(), nil),
		exp.Open(gomock.Any(), "controller").Return(s.DB(), nil),
		exp.Open(gomock.Any(), "controller").DoAndReturn(func(_ context.Context, _ string) (*sql.DB, error) {
			_, db := s.OpenDB(c)
			return db, nil
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

	w, err := s.newTrackedDBWorker(c, pingFn)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)
	s.ensureDBReplaced(c)

	// There is a race potential race with the composition here, because
	// although the ping func may return a new database, it is not instantly
	// set as the worker's DB reference. We need to give it a chance.
	// In-theatre this will be OK, because a DB in an error state recoverable
	// by reconnecting will be replaced within the default retry strategy's
	// backoff/repeat loop.
	timeout := time.After(time.Millisecond * 500)
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
}

func (s *trackedDBWorkerSuite) TestWorkerAttemptsToVerifyDBButFails(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()
	defer s.expectTimer(1)()

	s.dbApp.EXPECT().Open(gomock.Any(), "controller").Return(s.DB(), nil).Times(DefaultVerifyAttempts)

	pingFn := func(context.Context, *sql.DB) error {
		return errors.New("boom")
	}

	w, err := s.newTrackedDBWorker(c, pingFn)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	c.Assert(w.Wait(), tc.ErrorMatches, "boom")

	// Ensure that the DB is dead.
	err = w.StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		c.Fatal("failed if called")
		return nil
	})
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *trackedDBWorkerSuite) TestWorkerCancelsTxn(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()
	defer s.expectTimer(0)()

	s.dbApp.EXPECT().Open(gomock.Any(), "controller").Return(s.DB(), nil)

	w, err := s.newTrackedDBWorker(c, defaultPingDBFunc)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	// Ensure that the DB is dead.
	err = w.StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		w.Kill()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-c.Context().Done():
			c.Fatal("timed out waiting for context to be canceled")
		}
		return nil
	})

	c.Assert(err, tc.ErrorMatches, "context canceled")
}

func (s *trackedDBWorkerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := s.dbBaseSuite.setupMocks(c)

	// Ensure we buffer the channel, this is because we might miss the
	// event if we're too quick at starting up.
	s.states = make(chan string, 1)

	return ctrl
}

func (s *trackedDBWorkerSuite) newTrackedDBWorker(c *tc.C, pingFn func(context.Context, *sql.DB) error) (TrackedDB, error) {
	collector := NewMetricsCollector()
	return newTrackedDBWorker(c.Context(),
		s.states,
		s.dbApp, "controller",
		WithClock(s.clock),
		WithLogger(s.logger),
		WithPingDBFunc(pingFn),
		WithMetricsCollector(collector),
	)
}

func (s *trackedDBWorkerSuite) ensureStartup(c *tc.C) {
	select {
	case state := <-s.states:
		c.Assert(state, tc.Equals, stateStarted)
	case <-time.After(testing.ShortWait * 10):
		c.Fatalf("timed out waiting for startup")
	}
}

func (s *trackedDBWorkerSuite) ensureDBReplaced(c *tc.C) {
	select {
	case state := <-s.states:
		c.Assert(state, tc.Equals, stateDBReplaced)
	case <-time.After(testing.ShortWait * 10):
		c.Fatalf("timed out waiting for startup")
	}
}

func readTableNames(c *tc.C, w coredatabase.TxnRunner) []string {
	// Attempt to use the new db, note there shouldn't be any leases in this
	// db.
	var tables []string
	err := w.StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.Query("SELECT tbl_name FROM sqlite_schema")
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var table string
			err = rows.Scan(&table)
			if err != nil {
				return err
			}
			tables = append(tables, table)
		}

		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	return set.NewStrings(tables...).SortedValues()
}

type sliceContainsChecker[T comparable] struct {
	*tc.CheckerInfo
}

var SliceContains tc.Checker = &sliceContainsChecker[string]{
	&tc.CheckerInfo{Name: "SliceContains", Params: []string{"obtained", "expected"}},
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

type hasKeysChecker[T comparable] struct {
	*tc.CheckerInfo
}

var MapHasKeys tc.Checker = &hasKeysChecker[string]{
	&tc.CheckerInfo{Name: "hasKeysChecker", Params: []string{"obtained", "expected"}},
}

func (checker *hasKeysChecker[T]) Check(params []interface{}, names []string) (result bool, error string) {
	expected, ok := params[1].([]T)
	if !ok {
		var t T
		return false, fmt.Sprintf("expected must be %T", t)
	}

	obtained, ok := params[0].(map[T]any)
	if !ok {
		var t T
		return false, fmt.Sprintf("Obtained value is not a map[%T]any", t)
	}

	for _, k := range expected {
		if _, ok := obtained[k]; !ok {
			return false, fmt.Sprintf("expected key %v not found", k)
		}
	}
	return true, ""
}
