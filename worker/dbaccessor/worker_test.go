// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbaccessor

import (
	"context"
	sql "database/sql"
	"errors"
	"time"

	"github.com/golang/mock/gomock"
	clock "github.com/juju/clock"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/database/app"
	"github.com/juju/juju/testing"
)

type workerSuite struct {
	baseSuite

	nodeManager *MockNodeManager
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) TestGetControllerDBSuccessNotExistingNode(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()
	s.expectClock()

	done := s.expectTrackedDB(c)

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

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	getter, ok := w.(DBGetter)
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

	// If this is an existing node, we don't invoke
	// the address or cluster options.
	mgrExp.IsExistingNode().Return(true, nil)
	mgrExp.WithLogFuncOption().Return(nil)

	sync := make(chan struct{})

	appExp := s.dbApp.EXPECT()
	appExp.Ready(gomock.Any()).Return(nil)
	appExp.ID().DoAndReturn(func() uint64 {
		close(sync)
		return uint64(666)
	})
	appExp.Handover(gomock.Any()).Return(nil)
	appExp.Close().Return(nil)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	select {
	case <-sync:
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for synchronisation")
	}

	// Close the wait on the tracked DB
	close(done)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)
	s.nodeManager = NewMockNodeManager(ctrl)
	return ctrl
}

func (s *workerSuite) newWorker(c *gc.C) worker.Worker {
	cfg := WorkerConfig{
		NodeManager: s.nodeManager,
		Clock:       s.clock,
		Logger:      s.logger,
		NewApp: func(string, ...app.Option) (DBApp, error) {
			return s.dbApp, nil
		},
		NewDBWorker: func(DBApp, string, clock.Clock, Logger) (TrackedDB, error) {
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

	w, err := NewTrackedDBWorker(s.dbApp, "controller", s.clock, s.logger)
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

	w, err := NewTrackedDBWorker(s.dbApp, "controller", s.clock, s.logger)
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

	w, err := NewTrackedDBWorker(s.dbApp, "controller", s.clock, s.logger)
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

	s.dbApp.EXPECT().Open(gomock.Any(), "controller").Return(s.DB(), nil).Times(DefaultVerifyAttempts)

	w, err := NewTrackedDBWorker(s.dbApp, "controller", s.clock, s.logger)
	c.Assert(err, jc.ErrorIsNil)

	var count int
	s.setupVerifyDBFunc(c, w, func(db *sql.DB) error {
		defer func() { count++ }()

		if count == DefaultVerifyAttempts-1 {
			return nil
		}
		return errors.New("boom")
	})

	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for DB callback")
	}

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

	w, err := NewTrackedDBWorker(s.dbApp, "controller", s.clock, s.logger)
	c.Assert(err, jc.ErrorIsNil)

	s.setupVerifyDBFunc(c, w, func(db *sql.DB) error {
		return errors.New("boom")
	})

	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for DB callback")
	}

	c.Assert(w.Wait(), gc.ErrorMatches, "boom")
	c.Assert(w.Err(), gc.ErrorMatches, "boom")
}

func (s *trackedDBWorkerSuite) setupVerifyDBFunc(c *gc.C, w TrackedDB, fn func(db *sql.DB) error) {
	trackedDBWorker, ok := w.(*trackedDBWorker)
	c.Assert(ok, jc.IsTrue, gc.Commentf("expected *trackedDBWorker, got %T", w))
	trackedDBWorker.verifyDBFunc = fn
}
