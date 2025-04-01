// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package removal

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/watcher/watchertest"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type workerSuite struct {
	testing.IsolationSuite

	svc *MockRemovalService
	clk *MockClock
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) TestWorkerStartStop(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	ch := make(chan []string)
	watch := watchertest.NewMockStringsWatcher(ch)
	s.svc.EXPECT().WatchRemovals().Return(watch, nil)

	// Use the timer creation as a synchronisation point below.
	// so that we know we are entrant into the worker's loop.
	sync := make(chan struct{})
	s.clk.EXPECT().NewTimer(jobCheckMaxInterval).DoAndReturn(func(d time.Duration) clock.Timer {
		sync <- struct{}{}
		return clock.WallClock.NewTimer(d)
	})

	cfg := Config{
		RemovalService: s.svc,
		Clock:          s.clk,
		Logger:         loggertesting.WrapCheckLog(c),
	}

	w, err := NewWorker(cfg)
	c.Assert(err, jc.ErrorIsNil)

	select {
	case <-sync:
	case <-time.After(testing.ShortWait):
		c.Fatalf("timed out waiting for worker to start")
	}

	workertest.CleanKill(c, w)
}

func (s *workerSuite) setUpMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.svc = NewMockRemovalService(ctrl)
	s.clk = NewMockClock(ctrl)

	return ctrl
}
