// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package removal

import (
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
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) TestWorkerStartStop(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	ch := make(chan []string)
	watch := watchertest.NewMockStringsWatcher(ch)
	s.svc.EXPECT().WatchRemovals().Return(watch, nil)

	cfg := Config{
		RemovalService: s.svc,
		Logger:         loggertesting.WrapCheckLog(c),
	}

	w, err := NewWorker(cfg)
	c.Assert(err, jc.ErrorIsNil)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) setUpMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.svc = NewMockRemovalService(ctrl)
	return ctrl
}
