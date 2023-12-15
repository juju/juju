// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package filenotifywatcher

import (
	time "time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
)

type workerSuite struct {
	baseSuite
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) TestChanges(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAnyLogs()
	s.expectAnyClock()

	done := make(chan struct{})
	ch := make(chan bool)
	go func() {
		defer close(done)
		ch <- true
		ch <- true
	}()

	s.watcher.EXPECT().Changes().Return(ch).Times(2)
	s.watcher.EXPECT().Wait().MinTimes(1)

	// Depending on the timing of the test, the worker may or may not have
	// received the kill signal before the watcher is killed.
	s.watcher.EXPECT().Kill().AnyTimes()

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	watcher, ok := w.(FileNotifyWatcher)
	c.Assert(ok, jc.IsTrue, gc.Commentf("worker does not implement FileNotifyWatcher"))

	ch1, err := watcher.Changes("controller")
	c.Assert(err, jc.ErrorIsNil)

	select {
	case v := <-ch1:
		c.Assert(v, jc.IsTrue)
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for changes")
	}

	ch2, err := watcher.Changes("controller")
	c.Assert(err, jc.ErrorIsNil)

	select {
	case v := <-ch2:
		c.Assert(v, jc.IsTrue)
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for changes")
	}

	workertest.CleanKill(c, w)

	select {
	case <-done:
	case <-time.After(testing.ShortWait):
		c.Fatalf("timed out waiting for worker to exit")
	}
}

func (s *workerSuite) newWorker(c *gc.C) worker.Worker {
	cfg := WorkerConfig{
		Clock:  s.clock,
		Logger: s.logger,
		NewWatcher: func(string, ...Option) (FileWatcher, error) {
			return s.watcher, nil
		},
	}

	w, err := newWorker(cfg)
	c.Assert(err, jc.ErrorIsNil)
	return w
}
