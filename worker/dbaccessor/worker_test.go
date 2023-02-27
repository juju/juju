// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbaccessor

import (
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
		NewDBWorker: func(DBApp, string, FatalErrorChecker, clock.Clock, Logger) (TrackedDB, error) {
			return s.trackedDB, nil
		},
		FatalErrorChecker: IsFatalError,
	}

	w, err := newWorker(cfg)
	c.Assert(err, jc.ErrorIsNil)
	return w
}
