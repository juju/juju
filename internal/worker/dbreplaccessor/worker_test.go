// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbreplaccessor

import (
	"testing"

	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/database"
)

type workerSuite struct {
	baseSuite

	trackedDB *MockTrackedDB
	driver    *MockDriver
}

func TestWorkerSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &workerSuite{})
}

func (s *workerSuite) TestKilledGetDBErrDying(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	mgrExp := s.nodeManager.EXPECT()
	mgrExp.EnsureDataDir().Return(c.MkDir(), nil)
	mgrExp.DqliteSQLDriver(gomock.Any()).Return(s.driver, nil)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	dbw := w.(*dbReplWorker)
	ensureStartup(c, dbw)

	w.Kill()

	_, err := dbw.GetDB(c.Context(), "anything")
	c.Assert(err, tc.ErrorIs, database.ErrDBReplAccessorDying)
}

func (s *workerSuite) TestGetDB(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	mgrExp := s.nodeManager.EXPECT()
	mgrExp.EnsureDataDir().Return(c.MkDir(), nil)
	mgrExp.DqliteSQLDriver(gomock.Any()).Return(s.driver, nil)

	done := make(chan struct{})

	s.expectTrackedDB(done)

	w := s.newWorker(c)
	defer func() {
		close(done)
		workertest.DirtyKill(c, w)
	}()

	dbw := w.(*dbReplWorker)
	ensureStartup(c, dbw)

	runner, err := dbw.GetDB(c.Context(), "anything")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(runner, tc.NotNil)
}

func (s *workerSuite) TestGetDBNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.newDBReplWorker = func() (TrackedDB, error) {
		return nil, database.ErrDBNotFound
	}

	s.expectClock()

	mgrExp := s.nodeManager.EXPECT()
	mgrExp.EnsureDataDir().Return(c.MkDir(), nil)
	mgrExp.DqliteSQLDriver(gomock.Any()).Return(s.driver, nil)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	dbw := w.(*dbReplWorker)
	ensureStartup(c, dbw)

	_, err := dbw.GetDB(c.Context(), "other")

	// The error isn't passed through, although we really should expose this
	// in the runner.
	c.Assert(err, tc.ErrorMatches, `worker "other" not found`)
}

func (s *workerSuite) TestGetDBFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	mgrExp := s.nodeManager.EXPECT()
	mgrExp.EnsureDataDir().Return(c.MkDir(), nil)
	mgrExp.DqliteSQLDriver(gomock.Any()).Return(s.driver, nil)

	done := make(chan struct{})

	s.expectTrackedDB(done)

	w := s.newWorker(c)
	defer func() {
		close(done)
		workertest.DirtyKill(c, w)
	}()

	dbw := w.(*dbReplWorker)
	ensureStartup(c, dbw)

	runner, err := dbw.GetDB(c.Context(), "anything")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(runner, tc.NotNil)

	// Notice that no additional changes are expected.

	runner, err = dbw.GetDB(c.Context(), "anything")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(runner, tc.NotNil)
}

func (s *workerSuite) newWorker(c *tc.C) worker.Worker {
	return s.newWorkerWithDB(c, s.trackedDB)
}

func (s *workerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)

	s.trackedDB = NewMockTrackedDB(ctrl)
	s.driver = NewMockDriver(ctrl)

	return ctrl
}

func (s *workerSuite) expectTrackedDB(done chan struct{}) {
	s.trackedDB.EXPECT().Kill().AnyTimes()
	s.trackedDB.EXPECT().Wait().DoAndReturn(func() error {
		<-done
		return nil
	})
}
