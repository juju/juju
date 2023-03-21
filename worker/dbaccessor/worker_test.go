// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbaccessor

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/collections/set"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/database/app"
	"github.com/juju/juju/database/dqlite"
	"github.com/juju/juju/pubsub/apiserver"
	"github.com/juju/juju/testing"
)

type workerSuite struct {
	baseSuite

	nodeManager *MockNodeManager
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) TestGetControllerDBSuccessNotExistingNode(c *gc.C) {
	c.Skip("to be reinstated in follow-up patch")
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()
	s.expectClock()

	mgrExp := s.nodeManager.EXPECT()
	mgrExp.EnsureDataDir().Return(c.MkDir(), nil)
	mgrExp.IsExistingNode().Return(false, nil)
	mgrExp.WithAddressOption().Return(nil, nil)
	mgrExp.WithClusterOption().Return(nil, nil)
	mgrExp.WithLogFuncOption().Return(nil)

	appExp := s.dbApp.EXPECT()
	appExp.Ready(gomock.Any()).Return(nil)
	appExp.ID().Return(uint64(666))
	appExp.Handover(gomock.Any()).Return(nil)
	appExp.Close().Return(nil)

	done := s.expectTrackedDB(c)
	s.hub.EXPECT().Subscribe(apiserver.DetailsTopic, gomock.Any()).Return(func() {}, nil)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	getter, ok := w.(coredatabase.DBGetter)
	c.Assert(ok, jc.IsTrue, gc.Commentf("worker does not implement DBGetter"))

	_, err := getter.GetDB("controller")
	c.Assert(err, jc.ErrorIsNil)

	// Close the wait on the tracked DB
	close(done)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestWorkerStartupExistingNode(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()
	s.expectClock()

	done := s.expectTrackedDB(c)

	mgrExp := s.nodeManager.EXPECT()
	mgrExp.EnsureDataDir().Return(c.MkDir(), nil)

	// If this is an existing node, we do not invoke the address or cluster
	// options, but if the node is not as bootstrapped, we do assume it is
	// part of a cluster, and uses the TLS option.
	mgrExp.IsExistingNode().Return(true, nil)
	mgrExp.IsBootstrappedNode(gomock.Any()).Return(false, nil)
	mgrExp.WithLogFuncOption().Return(nil)
	mgrExp.WithTLSOption().Return(nil, nil)

	sync := make(chan struct{})

	appExp := s.dbApp.EXPECT()
	appExp.Ready(gomock.Any()).Return(nil)
	appExp.ID().DoAndReturn(func() uint64 {
		close(sync)
		return uint64(666)
	})
	appExp.Handover(gomock.Any()).Return(nil)
	appExp.Close().Return(nil)

	s.hub.EXPECT().Subscribe(apiserver.DetailsTopic, gomock.Any()).Return(func() {}, nil)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	select {
	case <-sync:
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for Dqlite node start")
	}

	// Close the wait on the tracked DB.
	close(done)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestWorkerStartupAsBootstrapNodeThenReconfigure(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()
	s.expectClock()

	done := s.expectTrackedDB(c)

	dataDir := c.MkDir()
	mgrExp := s.nodeManager.EXPECT()
	mgrExp.EnsureDataDir().Return(dataDir, nil).MinTimes(1)

	// If this is an existing node, we do not
	// invoke the address or cluster options.
	mgrExp.IsExistingNode().Return(true, nil).Times(2)
	mgrExp.IsBootstrappedNode(gomock.Any()).Return(true, nil).Times(2)
	mgrExp.WithLogFuncOption().Return(nil)

	// These are the expectations around reconfiguring
	// the cluster and local node.
	mgrExp.ClusterServers(gomock.Any()).Return([]dqlite.NodeInfo{
		{
			ID:      3297041220608546238,
			Address: "127.0.0.1:17666",
			Role:    0,
		},
	}, nil)
	mgrExp.SetClusterServers(gomock.Any(), []dqlite.NodeInfo{
		{
			ID:      3297041220608546238,
			Address: "10.6.6.6:17666",
			Role:    0,
		},
	}).Return(nil)
	mgrExp.SetNodeInfo(dqlite.NodeInfo{
		ID:      3297041220608546238,
		Address: "10.6.6.6:17666",
		Role:    0,
	}).Return(nil)

	sync := make(chan struct{})

	appExp := s.dbApp.EXPECT()
	appExp.Ready(gomock.Any()).Return(nil)
	appExp.ID().DoAndReturn(func() uint64 {
		close(sync)
		return uint64(666)
	})
	appExp.Handover(gomock.Any()).Return(nil)
	appExp.Close().Return(nil)

	s.hub.EXPECT().Subscribe(apiserver.DetailsTopic, gomock.Any()).Return(func() {}, nil)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	select {
	case <-sync:
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for Dqlite node start")
	}

	// At this point we have started successfully.
	// Push a message onto the API details channel to simulate a move into HA.
	select {
	case w.(*dbWorker).apiServerChanges <- apiserver.Details{
		Servers: map[string]apiserver.APIServer{
			"0": {ID: "0", InternalAddress: "10.6.6.6:1234"},
		},
	}:
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for cluster change to be processed")
	}

	// Close the wait on the tracked DB.
	close(done)

	err := workertest.CheckKilled(c, w)
	c.Assert(errors.Is(err, dependency.ErrBounce), jc.IsTrue)
}

func (s *workerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)
	s.nodeManager = NewMockNodeManager(ctrl)
	return ctrl
}

func (s *workerSuite) newWorker(c *gc.C) worker.Worker {
	cfg := WorkerConfig{
		NodeManager:  s.nodeManager,
		Clock:        s.clock,
		Hub:          s.hub,
		ControllerID: "0",
		Logger:       s.logger,
		NewApp: func(string, ...app.Option) (DBApp, error) {
			return s.dbApp, nil
		},
		NewDBWorker: func(DBApp, string, ...TrackedDBWorkerOption) (TrackedDB, error) {
			return s.trackedDB, nil
		},
	}

	w, err := newWorker(cfg)
	c.Assert(err, jc.ErrorIsNil)
	return w
}

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

	w, err := NewTrackedDBWorker(s.dbApp, "controller", WithClock(s.clock), WithLogger(s.logger))
	c.Assert(err, jc.ErrorIsNil)

	defer workertest.DirtyKill(c, w)

	done := make(chan struct{})
	w.DB(func(d *sql.DB) error {
		defer close(done)

		c.Assert(d, gc.NotNil)
		return nil
	})

	select {
	case <-done:
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for DB callback")
	}

	workertest.CleanKill(c, w)
}

func (s *trackedDBWorkerSuite) TestWorkerTxnIsNotNil(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()
	s.expectClock()
	s.expectTimer(0)

	s.dbApp.EXPECT().Open(gomock.Any(), "controller").Return(s.DB(), nil)

	w, err := NewTrackedDBWorker(s.dbApp, "controller", WithClock(s.clock), WithLogger(s.logger))
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

func (s *trackedDBWorkerSuite) TestWorkerAttemptsToVerifyDBButSucceeds(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()
	s.expectClock()
	done := s.expectTimer(1)

	s.logger.EXPECT().Errorf(gomock.Any(), gomock.Any()).AnyTimes()

	dbChange := make(chan struct{})
	s.dbApp.EXPECT().Open(gomock.Any(), "controller").Return(s.DB(), nil).Times(DefaultVerifyAttempts - 1)
	s.dbApp.EXPECT().Open(gomock.Any(), "controller").Return(s.DB(), nil).DoAndReturn(func(_ context.Context, _ string) (*sql.DB, error) {
		defer close(dbChange)
		return s.DB(), nil
	})

	var count uint64
	verifyFn := func(context.Context, *sql.DB) error {
		val := atomic.AddUint64(&count, 1)

		if val == DefaultVerifyAttempts {
			return nil
		}
		return errors.New("boom")
	}

	w, err := NewTrackedDBWorker(s.dbApp, "controller", WithClock(s.clock), WithLogger(s.logger), WithVerifyDBFunc(verifyFn))
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

	tables := checkTableNames(c, w)
	c.Assert(tables, SliceContains, "lease")

	workertest.CleanKill(c, w)

	c.Assert(w.Err(), jc.ErrorIsNil)
}

func (s *trackedDBWorkerSuite) TestWorkerAttemptsToVerifyDBButSucceedsWithDifferentDB(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()
	s.expectClock()
	done := s.expectTimer(1)

	s.logger.EXPECT().Errorf(gomock.Any(), gomock.Any()).AnyTimes()

	dbChange := make(chan struct{})
	s.dbApp.EXPECT().Open(gomock.Any(), "controller").Return(s.DB(), nil)
	s.dbApp.EXPECT().Open(gomock.Any(), "controller").Return(s.DB(), nil)
	s.dbApp.EXPECT().Open(gomock.Any(), "controller").DoAndReturn(func(_ context.Context, _ string) (*sql.DB, error) {
		defer close(dbChange)
		return s.NewCleanDB(c), nil
	})

	var count uint64
	verifyFn := func(context.Context, *sql.DB) error {
		val := atomic.AddUint64(&count, 1)

		if val == DefaultVerifyAttempts {
			return nil
		}
		return errors.New("boom")
	}

	w, err := NewTrackedDBWorker(s.dbApp, "controller", WithClock(s.clock), WithLogger(s.logger), WithVerifyDBFunc(verifyFn))
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

	tables := checkTableNames(c, w)
	c.Assert(tables, gc.Not(SliceContains), "lease")

	workertest.CleanKill(c, w)

	c.Assert(w.Err(), jc.ErrorIsNil)
}

func (s *trackedDBWorkerSuite) TestWorkerAttemptsToVerifyDBButFails(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()
	s.expectClock()
	done := s.expectTimer(1)

	s.logger.EXPECT().Errorf(gomock.Any(), gomock.Any()).AnyTimes()

	s.dbApp.EXPECT().Open(gomock.Any(), "controller").Return(s.DB(), nil).Times(DefaultVerifyAttempts)

	verifyFn := func(context.Context, *sql.DB) error {
		return errors.New("boom")
	}

	w, err := NewTrackedDBWorker(s.dbApp, "controller", WithClock(s.clock), WithLogger(s.logger), WithVerifyDBFunc(verifyFn))
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
	err = w.DB(func(db *sql.DB) error {
		c.Fatal("failed if called")
		return nil
	})
	c.Assert(err, gc.ErrorMatches, "boom")
	err = w.Txn(context.TODO(), func(ctx context.Context, tx *sql.Tx) error {
		c.Fatal("failed if called")
		return nil
	})
	c.Assert(err, gc.ErrorMatches, "boom")
}

func checkTableNames(c *gc.C, w coredatabase.TrackedDB) []string {
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
