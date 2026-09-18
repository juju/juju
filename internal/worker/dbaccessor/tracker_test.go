// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbaccessor

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"sync/atomic"
	stdtesting "testing"
	"time"

	"github.com/canonical/gomock/gomock"
	"github.com/canonical/sqlair"
	"github.com/juju/clock/testclock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v5/workertest"
	"github.com/mattn/go-sqlite3"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/internal/database/dqlite"
	"github.com/juju/juju/internal/testhelpers"
)

// Ensure that the trackedDBWorker is a killableWorker.
var _ killableWorker = (*trackedDBWorker)(nil)

type trackedDBWorkerSuite struct {
	dbBaseSuite

	states chan string
}

func TestTrackedDBWorkerSuite(t *stdtesting.T) {
	testhelpers.PrintGoroutineLeaks(t, func(t *stdtesting.T) {
		tc.Run(t, &trackedDBWorkerSuite{})
	})
}

func (s *trackedDBWorkerSuite) TestWorkerStartup(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()
	s.expectNotLeader()
	defer s.expectTimer(0)()

	s.dbApp.EXPECT().Open(gomock.Any(), "controller").Return(s.DB(), nil)

	w, err := NewTrackedDBWorker(c.Context(), s.dbApp, "controller", WithClock(s.clock), WithLogger(s.logger))
	c.Assert(err, tc.ErrorIsNil)

	workertest.CleanKill(c, w)
}

func (s *trackedDBWorkerSuite) TestWorkerReport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()
	s.expectNotLeader()
	defer s.expectTimer(0)()

	s.dbApp.EXPECT().Open(gomock.Any(), "controller").Return(s.DB(), nil)

	w, err := NewTrackedDBWorker(c.Context(), s.dbApp, "controller", WithClock(s.clock), WithLogger(s.logger))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	report := w.(interface {
		Report(ctx context.Context) map[string]any
	}).Report(c.Context())
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
	s.expectNotLeader()
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

func (s *trackedDBWorkerSuite) TestWorkerStdTxnNoRetry(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()
	s.expectNotLeader()
	defer s.expectTimer(0)()

	s.dbApp.EXPECT().Open(gomock.Any(), "controller").Return(s.DB(), nil)

	w, err := s.newTrackedDBWorker(c, defaultPingDBFunc)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	runner, ok := w.(interface {
		StdTxnNoRetry(context.Context, func(context.Context, *sql.Tx) error) error
	})
	c.Assert(ok, tc.IsTrue)

	var attempts atomic.Int64
	err = runner.StdTxnNoRetry(c.Context(), func(context.Context, *sql.Tx) error {
		attempts.Add(1)
		return sqlite3.ErrBusy
	})
	c.Check(err, tc.ErrorIs, sqlite3.ErrBusy)
	c.Check(attempts.Load(), tc.Equals, int64(1))

	workertest.CleanKill(c, w)
}

func (s *trackedDBWorkerSuite) TestWorkerTxnIsNotNil(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()
	s.expectNotLeader()
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
	case <-c.Context().Done():
		c.Fatal("timed out waiting for DB callback")
	}

	workertest.CleanKill(c, w)
}

func (s *trackedDBWorkerSuite) TestWorkerStdTxnIsNotNil(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()
	s.expectNotLeader()
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
	case <-c.Context().Done():
		c.Fatal("timed out waiting for DB callback")
	}

	workertest.CleanKill(c, w)
}

func (s *trackedDBWorkerSuite) TestWorkerRunReusesPreparedContextForRetries(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()
	s.expectNotLeader()
	defer s.expectTimer(0)()

	s.dbApp.EXPECT().Open(gomock.Any(), "controller").Return(s.DB(), nil)

	w, err := s.newTrackedDBWorker(c, defaultPingDBFunc)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	worker := w.(*trackedDBWorker)
	contexts := make(chan context.Context, 2)
	var attempts atomic.Int64
	err = worker.run(c.Context(), func(ctx context.Context, _ *sqlair.DB) error {
		contexts <- ctx
		if attempts.Add(1) == 1 {
			return sqlite3.ErrBusy
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	first := <-contexts
	second := <-contexts
	c.Check(first == second, tc.IsTrue)
	c.Check(attempts.Load(), tc.Equals, int64(2))

	workertest.CleanKill(c, w)
}

func (s *trackedDBWorkerSuite) TestWorkerAttemptsToVerifyDB(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// This test uses a dialated wall clock to test retries.
	s.clock = testclock.NewDilatedWallClock(time.Millisecond)

	s.expectNotLeader()

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
	s.expectNotLeader()
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
	case <-c.Context().Done():
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

	s.expectNotLeader()

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
	s.expectNotLeader()
	s.setupOptimizeTimer()
	timerCh := s.setupTimer(PollInterval)

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

	// The internal state channel is intentionally best-effort and has a single
	// buffer slot. Trigger the poll only after startup has been observed so the
	// replacement state cannot be dropped behind stateStarted.
	select {
	case timerCh <- time.Now():
	case <-c.Context().Done():
		c.Fatal("timed out waiting to trigger DB verification")
	}

	s.ensureDBReplaced(c)

	// There is a race potential race with the composition here, because
	// although the ping func may return a new database, it is not instantly
	// set as the worker's DB reference. We need to give it a chance.
	// In-theatre this will be OK, because a DB in an error state recoverable
	// by reconnecting will be replaced within the default retry strategy's
	// backoff/repeat loop.
	tables := readTableNames(c, w)
loop:
	for {
		select {
		case <-c.Context().Done():
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
	s.expectNotLeader()
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
	s.expectNotLeader()
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

func (s *trackedDBWorkerSuite) TestWorkerOptimizesDatabaseOnOpen(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()
	defer s.expectTimer(0)()

	// This node hosts the Dqlite leader, so opening the database refreshes
	// the query planner statistics.
	s.expectLeader()

	s.dbApp.EXPECT().Open(gomock.Any(), "controller").Return(s.DB(), nil)

	s.seedOptimizableTable(c)

	w, err := s.newTrackedDBWorker(c, defaultPingDBFunc)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	c.Check(analysedIndexes(c, s.DB()), SliceContains, "optimize_test_value_idx")

	workertest.CleanKill(c, w)
}

func (s *trackedDBWorkerSuite) TestWorkerDoesNotOptimizeDatabaseOnOpenWhenNotLeader(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()
	defer s.expectTimer(0)()

	// The leader lives on another node, which is the node that will do the
	// work. Doing it here as well would only contend for the writer slot.
	s.expectNotLeader()

	s.dbApp.EXPECT().Open(gomock.Any(), "controller").Return(s.DB(), nil)

	s.seedOptimizableTable(c)

	w, err := s.newTrackedDBWorker(c, defaultPingDBFunc)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	c.Check(analysedIndexes(c, s.DB()), tc.HasLen, 0)

	workertest.CleanKill(c, w)
}

func (s *trackedDBWorkerSuite) TestWorkerOptimizesDatabaseOnTimer(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()
	s.setupTimer(PollInterval)
	optimizeCh := s.setupOptimizeTimer()

	// The optimize timer is reset once the refresh has been attempted, so
	// resetting it tells us that the work is done.
	optimized := make(chan struct{})
	s.optimizeTimer.EXPECT().Reset(gomock.Any()).Times(1).DoAndReturn(func(time.Duration) bool {
		close(optimized)
		return true
	})

	// Not the leader when the database is opened, but the leader by the time
	// the refresh falls due, which isolates the work done on the timer.
	s.dbApp.EXPECT().Client(gomock.Any()).Return(s.client, nil).AnyTimes()
	s.dbApp.EXPECT().ID().Return(dqliteNodeID).AnyTimes()
	gomock.InOrder(
		s.client.EXPECT().Leader(gomock.Any()).Return(&dqlite.NodeInfo{ID: otherDqliteNodeID}, nil),
		s.client.EXPECT().Leader(gomock.Any()).Return(&dqlite.NodeInfo{ID: dqliteNodeID}, nil),
	)

	s.dbApp.EXPECT().Open(gomock.Any(), "controller").Return(s.DB(), nil)

	s.seedOptimizableTable(c)

	w, err := s.newTrackedDBWorker(c, defaultPingDBFunc)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	// Opening the database did nothing, because this node was not the leader.
	c.Assert(analysedIndexes(c, s.DB()), tc.HasLen, 0)

	s.tickOptimizeTimer(c, optimizeCh, optimized)

	c.Check(analysedIndexes(c, s.DB()), SliceContains, "optimize_test_value_idx")

	workertest.CleanKill(c, w)
}

func (s *trackedDBWorkerSuite) TestWorkerDoesNotOptimizeDatabaseOnTimerWhenNotLeader(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()
	s.setupTimer(PollInterval)
	optimizeCh := s.setupOptimizeTimer()

	optimized := make(chan struct{})
	s.optimizeTimer.EXPECT().Reset(gomock.Any()).Times(1).DoAndReturn(func(time.Duration) bool {
		close(optimized)
		return true
	})

	s.expectNotLeader()

	s.dbApp.EXPECT().Open(gomock.Any(), "controller").Return(s.DB(), nil)

	s.seedOptimizableTable(c)

	w, err := s.newTrackedDBWorker(c, defaultPingDBFunc)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)
	s.tickOptimizeTimer(c, optimizeCh, optimized)

	c.Check(analysedIndexes(c, s.DB()), tc.HasLen, 0)

	workertest.CleanKill(c, w)
}

func (s *trackedDBWorkerSuite) TestWorkerReportsOptimizations(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()
	s.setupTimer(PollInterval)
	optimizeCh := s.setupOptimizeTimer()

	optimized := make(chan struct{})
	s.optimizeTimer.EXPECT().Reset(gomock.Any()).Times(1).DoAndReturn(func(time.Duration) bool {
		close(optimized)
		return true
	})

	s.expectLeader()

	s.dbApp.EXPECT().Open(gomock.Any(), "controller").Return(s.DB(), nil)

	s.seedOptimizableTable(c)

	w, err := s.newTrackedDBWorker(c, defaultPingDBFunc)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)
	s.tickOptimizeTimer(c, optimizeCh, optimized)

	report := w.(interface {
		Report(ctx context.Context) map[string]any
	}).Report(c.Context())

	// Once when the database was opened, and once on the timer.
	c.Check(report["optimizations"], tc.Equals, uint32(2))

	workertest.CleanKill(c, w)
}

// tickOptimizeTimer fires the optimize timer and waits for the resulting
// refresh attempt to complete.
func (s *trackedDBWorkerSuite) tickOptimizeTimer(c *tc.C, tick chan time.Time, done chan struct{}) {
	select {
	case tick <- time.Now():
	case <-c.Context().Done():
		c.Fatal("timed out sending the optimize tick")
	}

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatal("timed out waiting for the database to be optimized")
	}
}

// seedOptimizableTable creates an indexed table with enough rows in it that
// the query planner statistics are worth gathering. PRAGMA optimize skips
// empty tables, so without the rows there would be nothing to analyse.
func (s *trackedDBWorkerSuite) seedOptimizableTable(c *tc.C) {
	db := s.DB()

	_, err := db.ExecContext(c.Context(), `
CREATE TABLE optimize_test (
	id    INTEGER PRIMARY KEY,
	value TEXT NOT NULL
)`)
	c.Assert(err, tc.ErrorIsNil)

	_, err = db.ExecContext(c.Context(), "CREATE INDEX optimize_test_value_idx ON optimize_test (value)")
	c.Assert(err, tc.ErrorIsNil)

	for i := range 100 {
		_, err = db.ExecContext(c.Context(),
			"INSERT INTO optimize_test (id, value) VALUES (?, ?)", i, fmt.Sprintf("value-%d", i%10))
		c.Assert(err, tc.ErrorIsNil)
	}
}

// analysedIndexes returns the names of the indexes that the query planner has
// gathered statistics for.
func analysedIndexes(c *tc.C, db *sql.DB) []string {
	// The statistics table only comes into existence the first time that
	// something is actually analysed.
	var analysed int
	err := db.QueryRowContext(c.Context(),
		"SELECT COUNT(*) FROM sqlite_master WHERE name = 'sqlite_stat1'").Scan(&analysed)
	c.Assert(err, tc.ErrorIsNil)

	if analysed == 0 {
		return nil
	}

	rows, err := db.QueryContext(c.Context(), "SELECT idx FROM sqlite_stat1 WHERE idx IS NOT NULL")
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = rows.Close() }()

	var indexes []string
	for rows.Next() {
		var index string
		c.Assert(rows.Scan(&index), tc.ErrorIsNil)
		indexes = append(indexes, index)
	}
	c.Assert(rows.Err(), tc.ErrorIsNil)

	return indexes
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
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for startup")
	}
}

func (s *trackedDBWorkerSuite) ensureDBReplaced(c *tc.C) {
	select {
	case state := <-s.states:
		c.Assert(state, tc.Equals, stateDBReplaced)
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for startup")
	}
}

func readTableNames(c *tc.C, w coredatabase.TxnRunner) []string {
	// Attempt to use the new db, note there shouldn't be any leases in this
	// db.
	var tables []string
	err := w.StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		tables = nil

		rows, err := tx.Query("SELECT tbl_name FROM sqlite_schema WHERE tbl_name NOT LIKE 'sqlite_%'")
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()

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

func (checker *sliceContainsChecker[T]) Check(params []any, names []string) (result bool, error string) {
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

	if slices.Contains(obtained, expected) {
		return true, ""
	}
	return false, ""
}

type hasKeysChecker[T comparable] struct {
	*tc.CheckerInfo
}

var MapHasKeys tc.Checker = &hasKeysChecker[string]{
	&tc.CheckerInfo{Name: "hasKeysChecker", Params: []string{"obtained", "expected"}},
}

func (checker *hasKeysChecker[T]) Check(params []any, names []string) (result bool, error string) {
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
