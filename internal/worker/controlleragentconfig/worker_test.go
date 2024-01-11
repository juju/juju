// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controlleragentconfig

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/juju/clock"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
)

type workerSuite struct {
	baseSuite
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) TestStartup(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w, _, states := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c, states)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestSighup(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w, notify, states := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c, states)

	s.sendSignal(c, notify)
	s.ensureReload(c, states)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestSighupMultipleTimes(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w, notify, states := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c, states)

	for i := 0; i < 10; i++ {
		s.sendSignal(c, notify)
		s.ensureReload(c, states)
	}

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestSighupAfterDeath(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w, notify, states := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c, states)

	workertest.CleanKill(c, w)

	// We should not receive a reload signal after the worker has died.
	s.sendSignal(c, notify)

	select {
	case state := <-states:
		c.Fatalf("should not have received state %q", state)
	case <-time.After(testing.ShortWait * 10):
	}
}

func (s *workerSuite) TestWatchWithNoChange(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w, _, states := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c, states)

	watcher, err := w.Watcher()
	c.Assert(err, jc.ErrorIsNil)
	defer watcher.Unsubscribe()

	changes := watcher.Changes()
	select {
	case <-changes:
		c.Fatal("should not have received a change")
	case <-time.After(testing.ShortWait * 10):
	}
}

func (s *workerSuite) TestWatchWithSubscribe(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w, notify, states := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c, states)

	watcher, err := w.Watcher()
	c.Assert(err, jc.ErrorIsNil)
	defer watcher.Unsubscribe()

	s.sendSignal(c, notify)
	s.ensureReload(c, states)

	changes := watcher.Changes()

	var count int
	select {
	case <-changes:
		count++
	case <-time.After(testing.ShortWait):
		c.Fatal("should have received a change")
	}

	c.Assert(count, gc.Equals, 1)

	select {
	case <-watcher.Done():
		c.Fatalf("should not have received a done signal")
	case <-time.After(testing.ShortWait):
	}

	workertest.CleanKill(c, w)

	ensureDone(c, watcher)
}

func (s *workerSuite) TestWatchAfterUnsubscribe(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w, notify, states := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c, states)

	watcher, err := w.Watcher()
	c.Assert(err, jc.ErrorIsNil)
	defer watcher.Unsubscribe()

	s.sendSignal(c, notify)
	s.ensureReload(c, states)

	watcher.Unsubscribe()

	changes := watcher.Changes()

	// The channel should be closed.
	select {
	case _, ok := <-changes:
		c.Assert(ok, jc.IsFalse)
	case <-time.After(testing.ShortWait * 10):
	}

	ensureDone(c, watcher)
}

func (s *workerSuite) TestWatchWithKilledWorker(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w, _, states := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c, states)

	watcher, err := w.Watcher()
	c.Assert(err, jc.ErrorIsNil)
	defer watcher.Unsubscribe()

	workertest.CleanKill(c, w)

	changes := watcher.Changes()

	select {
	case _, ok := <-changes:
		c.Assert(ok, jc.IsFalse)
	case <-time.After(testing.ShortWait * 10):
	}

	ensureDone(c, watcher)
}

func (s *workerSuite) TestWatchMultiple(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w, notify, states := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	s.ensureStartup(c, states)

	watchers := make([]ConfigWatcher, 10)
	for i := range watchers {
		watcher, err := w.Watcher()
		c.Assert(err, jc.ErrorIsNil)
		defer watcher.Unsubscribe()
		watchers[i] = watcher
	}

	s.sendSignal(c, notify)
	s.ensureReload(c, states)

	var wg sync.WaitGroup
	wg.Add(len(watchers))

	var count int64
	for i := 0; i < len(watchers); i++ {
		go func(w ConfigWatcher) {
			defer wg.Done()

			changes := w.Changes()
			select {
			case _, ok := <-changes:
				atomic.AddInt64(&count, 1)
				c.Assert(ok, jc.IsTrue)
			case <-time.After(testing.ShortWait * 10):
				c.Fatal("should have received a change")
			}
		}(watchers[i])
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for changes to finish")
	}

	c.Assert(atomic.LoadInt64(&count), gc.Equals, int64(len(watchers)))
}

func (s *workerSuite) TestWatchMultipleWithUnsubscribe(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w, notify, states := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	s.ensureStartup(c, states)

	watchers := make([]ConfigWatcher, 10)
	for i := range watchers {
		watcher, err := w.Watcher()
		c.Assert(err, jc.ErrorIsNil)
		watchers[i] = watcher
	}

	s.sendSignal(c, notify)
	s.ensureReload(c, states)

	var wg sync.WaitGroup
	wg.Add(len(watchers))

	var count int64
	for i := 0; i < len(watchers); i++ {
		go func(i int, w ConfigWatcher) {
			defer wg.Done()

			changes := w.Changes()

			// Test to ensure that a unsubscribe doesn't block another watcher.
			if (i % 2) == 0 {
				w.Unsubscribe()
				// Notice that we don't wait for the unsubscribe to complete.
				// Which means that the worker should not block sending
				// messages.
				return
			}

			select {
			case _, ok := <-changes:
				atomic.AddInt64(&count, 1)
				c.Assert(ok, jc.IsTrue)
			case <-time.After(testing.ShortWait * 10):
				c.Fatal("should have received a change")
			}
		}(i, watchers[i])
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for changes to finish")
	}

	c.Assert(atomic.LoadInt64(&count), gc.Equals, int64(len(watchers)/2))
}

func (s *workerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)
	return ctrl
}

func (s *workerSuite) newWorker(c *gc.C) (*configWorker, chan struct{}, chan string) {
	// Buffer the channel, so we don't drop signals if we're not ready.
	states := make(chan string, 10)
	// Buffer the channel, so we don't miss signals if we're not ready.
	notify := make(chan struct{}, 1)
	w, err := newWorker(WorkerConfig{
		Logger: s.logger,
		Clock:  clock.WallClock,
		Notify: func(ctx context.Context, ch chan os.Signal) {
			go func() {
				for {
					select {
					case <-notify:
						select {
						case ch <- syscall.SIGHUP:
						case <-ctx.Done():
							return
						}

					case <-ctx.Done():
						return
					}
				}
			}()
		},
	}, states)
	c.Assert(err, jc.ErrorIsNil)
	return w, notify, states
}

func (s *workerSuite) ensureStartup(c *gc.C, states chan string) {
	select {
	case state := <-states:
		c.Assert(state, gc.Equals, stateStarted)
	case <-time.After(testing.ShortWait * 10):
		c.Fatalf("timed out waiting for startup")
	}
}

func (s *workerSuite) ensureReload(c *gc.C, states chan string) {
	select {
	case state := <-states:
		c.Assert(state, gc.Equals, stateReload)
	case <-time.After(testing.ShortWait * 100):
		c.Fatalf("timed out waiting for reload")
	}
}

func (s *workerSuite) sendSignal(c *gc.C, notify chan struct{}) {
	select {
	case notify <- struct{}{}:
	case <-time.After(testing.ShortWait * 10):
		c.Fatalf("timed out sending signal")
	}
}

func ensureDone(c *gc.C, watcher ConfigWatcher) {
	select {
	case <-watcher.Done():
	case <-time.After(testing.ShortWait):
		c.Fatal("should have received a done signal")
	}
}
