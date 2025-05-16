// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package filenotifywatcher

import (
	stdtesting "testing"
	time "time"

	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"

	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testing"
)

type workerSuite struct {
	baseSuite
}

func TestWorkerSuite(t *stdtesting.T) { tc.Run(t, &workerSuite{}) }
func (s *workerSuite) TestChanges(c *tc.C) {
	defer s.setupMocks(c).Finish()

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
	c.Assert(ok, tc.IsTrue, tc.Commentf("worker does not implement FileNotifyWatcher"))

	ch1, err := watcher.Changes("controller")
	c.Assert(err, tc.ErrorIsNil)

	select {
	case v := <-ch1:
		c.Assert(v, tc.IsTrue)
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for changes")
	}

	ch2, err := watcher.Changes("controller")
	c.Assert(err, tc.ErrorIsNil)

	select {
	case v := <-ch2:
		c.Assert(v, tc.IsTrue)
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

func (s *workerSuite) newWorker(c *tc.C) worker.Worker {
	cfg := WorkerConfig{
		Clock:  s.clock,
		Logger: loggertesting.WrapCheckLog(c),
		NewWatcher: func(string, ...Option) (FileWatcher, error) {
			return s.watcher, nil
		},
	}

	w, err := newWorker(cfg)
	c.Assert(err, tc.ErrorIsNil)
	return w
}
