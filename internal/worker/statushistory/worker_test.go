// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statushistory

import (
	time "time"

	"github.com/juju/loggo/v2"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/status"
)

type workerSuite struct {
	baseSuite
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) TestStatusHistory(c *gc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()
	s.expectClock(now)

	s.logSink.EXPECT().GetLogger("foo").Return(s.logger)
	s.logger.EXPECT().Log([]logger.LogRecord{{
		Time:      now,
		Level:     loggo.INFO,
		ModelUUID: "foo",
		Entity:    "blah",
		Module:    "statushistory",
		Location:  "worker.go:102",
		Message:   "status history: application - unset status",
		Labels: map[string]string{
			"domain": "status",
			"kind":   "application",
			"id":     "blah",
			"status": "unset",
		},
	}}).Return(nil)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	worker := w.(*statusHistoryWorker)
	statusHistory := worker.StatusHistorySetterForModel("foo")
	err := statusHistory.SetStatusHistory(status.KindApplication, status.Unset, "blah")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *workerSuite) newWorker(c *gc.C) worker.Worker {
	w, err := NewWorker(Config{
		Clock:       s.clock,
		ModelLogger: s.logSink,
	})
	c.Assert(err, jc.ErrorIsNil)
	return w
}
