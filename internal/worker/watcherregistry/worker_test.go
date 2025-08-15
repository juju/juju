// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcherregistry

import (
	"fmt"
	"sync"
	stdtesting "testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"

	coreerrors "github.com/juju/juju/core/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/testing"
)

type registrySuite struct {
	testhelpers.IsolationSuite
}

func TestRegistrySuite(t *stdtesting.T) {
	tc.Run(t, &registrySuite{})
}

func (s *registrySuite) TestRegisterCount(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	reg, regGetter := s.newRegistry(c)
	defer workertest.CleanKill(c, regGetter)

	c.Check(reg.Count(), tc.Equals, 0)
}

func (s *registrySuite) TestRegisterGetCount(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	reg, regGetter := s.newRegistry(c)
	defer workertest.CleanKill(c, regGetter)

	for i := range 10 {
		w := workertest.NewErrorWorker(nil)

		id, err := reg.Register(c.Context(), w)
		c.Assert(err, tc.ErrorIsNil)

		w1, err := reg.Get(id)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(w1, tc.Equals, w)
		c.Check(reg.Count(), tc.Equals, i+1)
	}

	err := reg.StopAll()
	c.Assert(err, tc.ErrorIsNil)

	c.Check(reg.Count(), tc.Equals, 0)
}

func (s *registrySuite) TestRegisterNamedGetCount(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	reg, regGetter := s.newRegistry(c)
	defer workertest.CleanKill(c, regGetter)

	for i := range 10 {
		w := workertest.NewErrorWorker(nil)

		id := fmt.Sprintf("id-%d", i)
		err := reg.RegisterNamed(c.Context(), id, w)
		c.Assert(err, tc.ErrorIsNil)

		w1, err := reg.Get(id)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(w1, tc.Equals, w)
		c.Check(reg.Count(), tc.Equals, i+1)
	}

	err := reg.StopAll()
	c.Assert(err, tc.ErrorIsNil)

	c.Check(reg.Count(), tc.Equals, 0)
}

func (s *registrySuite) TestRegisterNamedRepeatedError(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	reg, regGetter := s.newRegistry(c)
	defer workertest.CleanKill(c, regGetter)

	w := workertest.NewErrorWorker(nil)

	err := reg.RegisterNamed(c.Context(), "foo", w)
	c.Assert(err, tc.ErrorIsNil)

	err = reg.RegisterNamed(c.Context(), "foo", w)
	c.Assert(err, tc.ErrorMatches, `worker "foo" already exists`)
	c.Assert(err, tc.ErrorIs, coreerrors.AlreadyExists)
}

func (s *registrySuite) TestRegisterNamedIntegerName(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	reg, regGetter := s.newRegistry(c)
	defer workertest.CleanKill(c, regGetter)

	w := workertest.NewErrorWorker(nil)

	err := reg.RegisterNamed(c.Context(), "0", w)
	c.Assert(err, tc.ErrorMatches, `namespace "0" not valid`)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *registrySuite) TestRegisterStop(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	reg, regGetter := s.newRegistry(c)
	defer workertest.CleanKill(c, regGetter)

	done := make(chan struct{})
	w := NewMockWorker(ctrl)
	w.EXPECT().Kill().DoAndReturn(func() {
		close(done)
	})
	w.EXPECT().Wait().DoAndReturn(func() error {
		select {
		case <-done:
		case <-time.After(testing.LongWait):
			c.Fatalf("timed out waiting for worker to finish")
		}

		return nil
	}).MinTimes(1)

	err := reg.RegisterNamed(c.Context(), "foo", w)
	c.Assert(err, tc.ErrorIsNil)

	err = reg.Stop("foo")
	c.Assert(err, tc.ErrorIsNil)

	c.Check(reg.Count(), tc.Equals, 0)
}

func (s *registrySuite) TestConcurrency(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// This test is designed to cause the race detector
	// to fail if the locking is not done correctly.
	reg, regGetter := s.newRegistry(c)
	defer workertest.CleanKill(c, regGetter)

	var wg sync.WaitGroup
	start := func(f func()) {
		wg.Add(1)
		go func() {
			f()
			wg.Done()
		}()
	}
	_, err := reg.Register(c.Context(), s.expectSimpleWatcher(ctrl))
	c.Assert(err, tc.ErrorIsNil)

	start(func() {
		_, _ = reg.Register(c.Context(), s.expectSimpleWatcher(ctrl))
	})
	start(func() {
		_ = reg.RegisterNamed(c.Context(), "named", s.expectSimpleWatcher(ctrl))
	})
	start(func() {
		_ = reg.Stop("1")
	})
	start(func() {
		_ = reg.Count()
	})
	start(func() {
		_ = reg.StopAll()
	})
	start(func() {
		_, _ = reg.Get("2")
	})
	start(func() {
		_, _ = reg.Get("named")
	})
	wg.Wait()
}

func (s *registrySuite) newRegistry(c *tc.C) (WatcherRegistry, worker.Worker) {
	w, err := NewWorker(Config{
		Clock:  clock.WallClock,
		Logger: loggertesting.WrapCheckLog(c),
	})
	c.Assert(err, tc.ErrorIsNil)

	reg, err := w.(*Worker).GetWatcherRegistry(c.Context(), 0)
	c.Assert(err, tc.ErrorIsNil)

	return reg, w
}

func (s *registrySuite) setupMocks(c *tc.C) *gomock.Controller {
	return gomock.NewController(c)
}

func (s *registrySuite) expectSimpleWatcher(ctrl *gomock.Controller) worker.Worker {
	w := NewMockWorker(ctrl)
	w.EXPECT().Kill().AnyTimes()
	w.EXPECT().Wait().DoAndReturn(func() error {
		<-time.After(testing.ShortWait)
		return nil
	}).AnyTimes()
	return w
}
