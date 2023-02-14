// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package changestream

import (
	"database/sql"

	"github.com/juju/clock"
	"github.com/juju/juju/core/changestream"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"
)

type workerSuite struct {
	baseSuite
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) TestChanges(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()

	changes := make(chan changestream.ChangeEvent)
	defer close(changes)

	s.dbGetter.EXPECT().GetDB("controller").Return(&sql.DB{}, nil)
	s.dbStream.EXPECT().Changes().Return(changes)
	s.dbStream.EXPECT().Wait().Return(nil).MinTimes(1)
	s.dbStream.EXPECT().Kill().MinTimes(1)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	stream, ok := w.(ChangeStream)
	c.Assert(ok, jc.IsTrue, gc.Commentf("worker does not implement ChangeStream"))

	_, err := stream.Changes("controller")
	c.Assert(err, jc.ErrorIsNil)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) newWorker(c *gc.C) worker.Worker {
	cfg := WorkerConfig{
		DBGetter: s.dbGetter,
		Clock:    s.clock,
		Logger:   s.logger,
		NewStream: func(*sql.DB, clock.Clock, Logger) DBStream {
			return s.dbStream
		},
	}

	w, err := newWorker(cfg)
	c.Assert(err, jc.ErrorIsNil)
	return w
}
